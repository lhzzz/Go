package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type RETCODE int

const (
	CFGPATH             string  = "./netCfg.ini"
	FILEPATH            string  = "./FILE/"
	VIEWPATH            string  = "./VIEW/"
	SUCCESS_RETUTN      RETCODE = 0
	INVALID_OPERATION   RETCODE = 1001
	UNSUPPORT_OPERATION RETCODE = 1002
	SOURCE_NOT_FOUND    RETCODE = 1003
	BODY_FORMAT_ERROR   RETCODE = 1004
)

type HandlerFunc func(http.ResponseWriter, *http.Request)
type MethodHnd struct {
	FnHanlder HandlerFunc
	RespBody  string
}
type TestReq struct {
	Uri string
}
type TestResp struct {
	MethodMap map[string]MethodHnd // method - handler
	IsDelete  bool                 //接口是否被删除
}

var priRouteList map[string]bool
var routeList map[TestReq]TestResp //路由对应的测试返回报文+方法
type TestReqJson struct {
	Uri    string `json:"uri"`
	Method string `json:"method"`
	Body   string `json:"respBody"`
}

func init() {
	routeList = make(map[TestReq]TestResp)
	priRouteList = make(map[string]bool)
	if checkFileIsExist(CFGPATH) {
		//do nothing
	} else {
		f, err := os.Create(CFGPATH) //创建文件
		defer f.Close()
		if err != nil {
			fmt.Printf("[ERROR]%v\n", err)
			return
		}
		_, err1 := io.WriteString(f, "port=8080\n") //写入文件(字符串)
		if err1 != nil {
			fmt.Printf("[ERROR]%v\n", err1)
			return
		}
	}
	if !checkFileIsExist(FILEPATH) {
		os.Mkdir(FILEPATH, os.ModePerm)
	}
	if !checkFileIsExist(VIEWPATH) {
		os.Mkdir(VIEWPATH, os.ModePerm)
	}
	priRouteList["/Main"] = true
	priRouteList["/Upload"] = true
	priRouteList["/VIEW"] = true
	priRouteList["/FILE"] = true
}
func checkFileIsExist(filename string) bool {
	var exist = true
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		exist = false
	}
	return exist
}

//登陆中间件
func AcsLog(f http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {

		fmt.Printf("Time: \033[1;35;40m%s\033[0m | IP: \033[1;36;40m%v\033[0m | Method: \033[1;34;40m%v\033[0m | Path: \033[1;32;40m%s\033[0m | Query: %s\n",
			time.Now().Format("2006-01-02 15:04:05"), req.RemoteAddr, req.Method, req.RequestURI, req.URL.RawQuery)

		if req.RequestURI != "/Upload" {
			// 把request的内容读取出来
			var bodyBytes []byte
			if req.Body != nil {
				bodyBytes, _ = ioutil.ReadAll(req.Body)
			}
			// 把刚刚读出来的再写进去
			req.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
			fmt.Printf("Body:\n\033[1;37;40m%v\033[0m\n", string(bodyBytes))
		}

		f(w, req)
	}
}
func JsonPrint(jsonBytes []byte) string {
	var out bytes.Buffer
	err := json.Indent(&out, jsonBytes, "", "\t")
	if err != nil {
		fmt.Printf("[ERROR]error:%v\n", err)
		return ""
	}
	return out.String()
}
func errJson(write http.ResponseWriter, code RETCODE, msg string) {
	type Ret struct {
		Code RETCODE
		Msg  string
	}
	var ret Ret

	ret.Code = code
	ret.Msg = msg
	retJson, err := json.Marshal(ret)
	if err != nil {
		fmt.Printf("[ERROR]error:%v\n", err)
	}
	io.WriteString(write, JsonPrint(retJson))
	return
}
func CreateNewRoute(write http.ResponseWriter, req *http.Request, reqJson TestReqJson) {
	testReq := TestReq{
		Uri: reqJson.Uri,
		//Method: reqJson.Method,
	}
	var mthdHnd MethodHnd
	//私有uri不可创建
	for priUri := range priRouteList {
		if strings.Contains(reqJson.Uri, priUri) {
			errJson(write, INVALID_OPERATION, "Invalid Operation:"+priUri+" route can not been operation")
			return
		}
	}
	//路由表中是否有此uri记录
	Resp, uriExist := routeList[testReq]
	if uriExist {
		//此uri是否有对应的method响应
		if _, ok := Resp.MethodMap[reqJson.Method]; ok {
			errJson(write, INVALID_OPERATION, "Invalid Operation:"+reqJson.Method+" "+reqJson.Uri+" is Exist, Need Update Operate")
			return
		}

	}
	mthdHnd.RespBody = reqJson.Body
	uriHandler := func(w http.ResponseWriter, r *http.Request) {
		var tReq TestReq
		tReq.Uri = trimpUri(r.RequestURI)
		rep, ok := routeList[tReq]
		if ok {
			if mdHnd, isMtdExist := rep.MethodMap[r.Method]; isMtdExist {
				io.WriteString(w, mdHnd.RespBody)
				return
			}
		}
		w.WriteHeader(404)
		errJson(w, SOURCE_NOT_FOUND, "404 Page Not Found")
		return
	}
	mthdHnd.FnHanlder = uriHandler
	// uriExist为false代表没有此路由，需要建立
	if !uriExist {
		Resp.MethodMap = make(map[string]MethodHnd)
		http.HandleFunc(reqJson.Uri, AcsLog(uriHandler)) // 配置新路由
	}
	Resp.MethodMap[reqJson.Method] = mthdHnd
	routeList[testReq] = Resp
	errJson(write, SUCCESS_RETUTN, "Operation Success")
	return
}
func GetSingleUriInfo(w http.ResponseWriter, req *http.Request) {
	var uri string
	var method string
	query := req.URL.Query()
	for key, item := range query {
		if key == "uri" {
			uri = item[0]
		} else if key == "method" {
			method = item[0]
		}
	}
	//fmt.Printf("Search uri:%s method:%s\n", uri, method)
	testReq := TestReq{
		Uri: uri,
	}
	Resp, ok := routeList[testReq]
	if ok {
		if mtdHnd, isMtdExist := Resp.MethodMap[method]; isMtdExist {
			UriInfo := TestReqJson{
				Uri:    uri,
				Method: method,
				Body:   mtdHnd.RespBody,
			}

			UriInfoJson, err := json.Marshal(UriInfo)
			if err != nil {
				fmt.Printf("[ERROR]error:%v\n", err)
				w.WriteHeader(500)
				return
			}

			io.WriteString(w, JsonPrint(UriInfoJson))
			return
		}
	}
	errJson(w, SOURCE_NOT_FOUND, "Uri or Method Not Found")
	return
}
func GetAllUriInfo(w http.ResponseWriter, req *http.Request) {
	var ArrayTestReq []TestReqJson

	//路由表搜索此uri对应的map集合
	for stReq, stResp := range routeList {
		for methodKey, mthHnd := range stResp.MethodMap {
			var UriInfo TestReqJson
			UriInfo.Uri = stReq.Uri
			UriInfo.Method = methodKey
			UriInfo.Body = mthHnd.RespBody
			ArrayTestReq = append(ArrayTestReq, UriInfo)
		}
	}
	retJson, err := json.Marshal(ArrayTestReq)
	if err != nil {
		w.WriteHeader(500)
		fmt.Printf("[ERROR]Make Json Error:%v\n", err)
		return
	}
	io.WriteString(w, JsonPrint(retJson))
	return
}
func GetRoute(w http.ResponseWriter, req *http.Request) {
	uri := req.RequestURI
	if strings.Contains(uri, "?") {
		GetSingleUriInfo(w, req)
	} else {
		GetAllUriInfo(w, req)
	}
}
func UpdateRoute(w http.ResponseWriter, req *http.Request, reqJson TestReqJson) {
	var testReq TestReq
	testReq.Uri = reqJson.Uri
	Resp, ok := routeList[testReq]
	if ok {
		//查看uri对应的method-Map中 方法是否存在
		if mtdHnd, isMtdExist := Resp.MethodMap[reqJson.Method]; isMtdExist {
			mtdHnd.RespBody = reqJson.Body
			Resp.MethodMap[reqJson.Method] = mtdHnd
			routeList[testReq] = Resp
			errJson(w, SUCCESS_RETUTN, "Operation Success")
			return
		}
	}
	errJson(w, SOURCE_NOT_FOUND, "Update Error, Uri or Method Not Found")
	return
}
func DeleteRoute(w http.ResponseWriter, req *http.Request) {
	var uri string
	var method string
	query := req.URL.Query()
	for key, item := range query {
		if key == "uri" {
			uri = item[0]
		} else if key == "method" {
			method = item[0]
		}
	}
	testReq := TestReq{
		Uri: uri,
	}
	resp, ok := routeList[testReq]
	if ok {
		if _, isMtdExist := resp.MethodMap[method]; isMtdExist {
			delete(resp.MethodMap, method)
			errJson(w, SUCCESS_RETUTN, "Operation Success")
			return
		}

	}

	errJson(w, SOURCE_NOT_FOUND, "Delete Error, Uri or Method Not Found")
	return
}

/*
TEST接口说明：
JSON：
{
 method:"POST","PUT","GET",
 uri:"/api/eventUpload",
 respBody:"xxx"
}
POST：创建测试路由和响应报文，在test路由下创建 ，如果同名且方法一样，则返回错误
PUT ：更新已存在测试路由的响应报文和方法
GET : 获取当前所有接口
GET?uri=xxx : 获取此uri对应的报文和方法
DELETE?uri=xxx： 删除此路由
*/
func Test(w http.ResponseWriter, req *http.Request) {
	var reqJson TestReqJson

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		fmt.Printf("[ERROR]Read Body err, %v\n", err)
		errJson(w, BODY_FORMAT_ERROR, err.Error())
		return
	}
	if len(body) > 0 {
		err = json.Unmarshal([]byte(body), &reqJson)
		if err != nil {
			fmt.Println("[ERROR]Parse Json Error", err)
			errJson(w, BODY_FORMAT_ERROR, err.Error())
			return
		}

		if reqJson.Method != "POST" &&
			reqJson.Method != "PUT" &&
			reqJson.Method != "GET" {
			errJson(w, BODY_FORMAT_ERROR, "Format Error:Not Support Method,Please use [POST,PUT,GET]")
			return
		}
	}

	reqJson.Uri = trimpUri(reqJson.Uri)
	//fmt.Printf("Req Json:method:%s,uri:%s,resp:%s\n", reqJson.Method, reqJson.Uri, reqJson.Body)
	if req.Method == "POST" {
		CreateNewRoute(w, req, reqJson)
	} else if req.Method == "GET" {
		GetRoute(w, req)
	} else if req.Method == "PUT" {
		UpdateRoute(w, req, reqJson)
	} else if req.Method == "DELETE" {
		DeleteRoute(w, req)
	} else {
		errJson(w, BODY_FORMAT_ERROR, "Format Error:Unsupport Method")
	}
	return
}
func trimpUri(uri string) string {
	index := strings.Index(uri, "?")
	if index < 0 {
		return uri
	}
	res := strings.TrimSpace(uri[:index])

	return res
}

//读取key=value类型的配置文件
func InitConfig(path string) map[string]string {
	config := make(map[string]string)
	f, err := os.Open(path)
	defer f.Close()
	if err != nil {
		panic(err)
	}
	r := bufio.NewReader(f)
	for {
		b, _, err := r.ReadLine()
		if err != nil {
			if err == io.EOF {
				break
			}
			panic(err)
		}
		s := strings.TrimSpace(string(b))
		index := strings.Index(s, "=")
		if index < 0 {
			continue
		}
		key := strings.TrimSpace(s[:index])
		if len(key) == 0 {
			continue
		}
		value := strings.TrimSpace(s[index+1:])
		if len(value) == 0 {
			continue
		}
		config[key] = value
	}
	return config
}

/*--------------------------------------------------------------WebFileServer---------------------------------------------------------------------------*/
func uploadHandle(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	host := strings.Split(r.Host, ":")
	ip := host[0]
	config := InitConfig(CFGPATH)
	port := config["port"]
	if r.Method == "POST" {
		//把上传的文件存储在内存和临时文件中
		r.ParseMultipartForm(32 << 20)
		//获取文件句柄，然后对文件进行存储等处理
		file, handler, err := r.FormFile("uploadfile")
		if err != nil {
			fmt.Println("form file err: ", err)
			return
		}
		defer file.Close()
		fmt.Fprintf(w, "<a href=\"http://%s:%s/FILE/%s\">http://%s:%s/FILE/%s</a>", ip, port, handler.Filename, ip, port, handler.Filename)
		//创建上传的目的文件
		f, err := os.OpenFile(FILEPATH+handler.Filename, os.O_WRONLY|os.O_CREATE, 0666)
		if err != nil {
			fmt.Println("open file err: ", err)
			return
		}
		defer f.Close()
		//拷贝文件
		io.Copy(f, file)
	}
}
func main() {
	config := InitConfig(CFGPATH)
	port := config["port"]
	if 0 == len(port) {
		port = "8080"
	}
	rPort := ":" + port
	http.HandleFunc("/Main", AcsLog(Test))
	http.HandleFunc("/Upload", AcsLog(uploadHandle)) //设置路由
	http.Handle("/FILE/", http.StripPrefix("/FILE/", http.FileServer(http.Dir(FILEPATH))))

	http.HandleFunc("/VIEW/upload.html", AcsLog(func(res http.ResponseWriter, req *http.Request) {
		host := strings.Split(req.Host, ":")
		ip := host[0]

		t, err := template.ParseFiles("./VIEW/upload.html")
		if err != nil {
			log.Println("err:", err)
			return
		}
		upUri := fmt.Sprintf("http://%s:%s/Upload", ip, port)
		t.Execute(res, upUri)
	}))

	//http.Handle("/VIEW/", http.StripPrefix("/VIEW/", http.FileServer(http.Dir(VIEWPATH))))

	fmt.Printf("[INFO]Server Init Success! Listen Port: %s\n", port)
	err := http.ListenAndServe(rPort, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
