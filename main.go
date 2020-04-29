package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	goldmarkHtml "github.com/yuin/goldmark/renderer/html"
)

type Config struct {
	Handler map[string]string `json:"handler"`
}

func (this *Config) Read(r io.Reader) error {
	bin, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	return json.Unmarshal(bin, this)
}

type Handler struct {
	Config   Config
	workDir  string
	notFound http.Handler
}

var fileServeSuffix = map[string]string{
	".gif":  "image/gif",
	".jpg":  "image/jpg",
	".png":  "image/jpg",
	".html": "text/html",
}

var markdownReader = goldmark.New(
	goldmark.WithRendererOptions(goldmarkHtml.WithUnsafe()))

func findPathInsteadOfDirectory(dir string) string {
	for _, fname := range []string{"index.html", "readme.md"} {
		path := filepath.Join(dir, fname)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func (this *Handler) serveHttp(w http.ResponseWriter, req *http.Request) error {
	log.Printf("%s %s %s\n", req.RemoteAddr, req.Method, req.URL.Path)
	targetPath := filepath.Join(this.workDir, filepath.FromSlash(req.URL.Path))
	stat, err := os.Stat(targetPath)
	if err != nil {
		return err
	}
	if stat.IsDir() {
		targetPath = findPathInsteadOfDirectory(targetPath)
		if targetPath == "" {
			return err
		}
	}

	suffix := path.Ext(targetPath)
	if interpreter, ok := this.Config.Handler[suffix]; ok {
		interpreter = filepath.FromSlash(interpreter)
		log.Printf("\"%s\" \"%s\"\n", interpreter, targetPath)
		if err := callCgi(interpreter, targetPath, w, req, os.Stderr, os.Stderr); err != nil {
			return err
		}
		return nil
	}
	if contentType, ok := fileServeSuffix[suffix]; ok {
		fd, err := os.Open(targetPath)
		if err != nil {
			return err
		}
		defer fd.Close()
		w.Header().Add("Content-Type", contentType)
		if stat, err := fd.Stat(); err == nil {
			w.Header().Add("Content-Length", strconv.FormatInt(stat.Size(), 10))
		}
		w.WriteHeader(http.StatusOK)
		io.Copy(w, fd)
		return nil
	}
	if strings.EqualFold(suffix, ".md") {
		source, err := ioutil.ReadFile(targetPath)
		if err != nil {
			return err
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `<html>
<head>
<style type="text/css"><!--
div.menubar{
    height:1.5em;
}
div.menubar div{
    position:absolute;
    z-index:100;
}
ul.mainmenu{
    margin:0px;
    padding:0px;
    width:100%;
    position:relative;
    list-style:none;
    text-align:center;
}
li.menuoff{
    position:relative;
    float:left;
    height:1.5em;
    line-height:1.5em;
    overflow:hidden;
    padding-left:1pt;
    padding-right:1pt;
}
li.menuon{
    float:left;
    background-color:white;
    line-height:1.5em;
    overflow:hidden;
    border-width:1px;border-color:black;border-style:solid;
    padding-left:1pt;
    padding-right:1pt;
}
ul.mainmenu>li.menuon{
    overflow:visible;
}
ul.submenu{
    margin:0px;
    padding:0px;
    position:relative;
    list-style:none;
}
.bqh1,.bqh2,.bqh3{
    font-weight:bold;
}
a.page_not_found{
    color:red;
}
p.centering,big{ font-size:200% }
h1{border-width:0px 0px 1px 0px;border-style:solid; border-color:#EAECEF}
h2{background-color:#CFC}
h3{border-width:0px 1px 1px 0px;border-style:solid}
h4{border-width:0 0 0 3mm;border-style:solid;border-color:#BFB;padding-left:1mm}
dt,span.commentator{font-weight:bold;padding:1mm}
span.comment_date{font-style:italic}
a{ text-decoration:none }
a:link{ color:green }
a:visited{ color:darkgreen }
a:hover{ text-decoration:underline }
pre,blockquote{ background-color:#F6F8FA ; padding:2mm }
table.block{ margin-left:1cm ; border-collapse: collapse;}
table.block th,table.block td{ border:solid 1px gray;padding:1pt}
pre{
 margin: 5mm;
 white-space: -moz-pre-wrap; /* Mozilla */
 white-space: -o-pre-wrap; /* Opera 7 */
 white-space: pre-wrap; /* CSS3 */
 word-wrap: break-word; /* IE 5.5+ */
}
div.tag{  text-align:right }
a.tag{ font-size:80%; background-color:#CFC }
span.tagnum{ font-size:70% ; color:green }
span.frozen{ font-size:80% ; color:#080 ; font-weight:bold }
@media screen{
 div.sidebar{ float:right; width:25% ; word-break: break-all;font-size:90%}
 div.main{ float:left; width:70% }
}
@media print{
 div.sidebar,div.footer,div.adminmenu{ display:none }
 div.main{ width:100% }
}
// -->
</style>
</head><body>`)
		err = markdownReader.Convert(source, w)

		fmt.Fprintln(w, `<hr />
Generated by <a href="https://github.com/zetamatta/xnhttpd">xnhttpd</a>
Powered by <a href="https://github.com/yuin/goldmark">goldmark</a>
</body></html>`)
		return err
	}
	return fmt.Errorf("%s: not support suffix", suffix)
}

func (this *Handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if err := this.serveHttp(w, req); err != nil {
		this.notFound.ServeHTTP(w, req)
		log.Printf("%s\n", err.Error())
	}
}

func mains(args []string) error {
	var handler Handler
	err := handler.Config.Read(os.Stdin)
	if err != nil {
		return err
	}
	handler.notFound = http.NotFoundHandler()
	handler.workDir, err = os.Getwd()
	if err != nil {
		return err
	}
	service := &http.Server{
		Addr:           ":8000",
		Handler:        &handler,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	err = service.ListenAndServe()
	closeErr := service.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func main() {
	if err := mains(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
