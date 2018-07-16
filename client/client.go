package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/keroserene/go-webrtc"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo"
	"ubox.golib/p2p/protocol"
	"webRtc/proxy"
	"encoding/base64"
)

type SdpReq struct {
	Box_id  string `json:"box_id"`
	Action  int    `json:"action"`
	App_sdp string `json:"app_sdp"`
}

type SdpRsp struct {
	Err_no  int    `json:"err_no"`
	Err_msg string `json:"err_msg"`
	Box_sdp string `json:"box_sdp"`
}

const BOXID = "123"

var (
	ChOnGenerateOffer = make(chan int, 1)
	ChSignalRegister  = make(chan string, 1)
	//ChStartGetBoxSdp  = make(chan string, 1)
	ChStartRegAppSdp  = make(chan string, 1)
	ChAnswerSdpReady = make(chan string, 1)
	ChDcEnable = make(chan string, 1)
	ChAllOk = make(chan int, 1)

	dc *webrtc.DataChannel
)

func mainprocess() {


	// Step 1. create pc
	pc := createpc()

	// Step 2. register callback
	registerCallback(pc, ChOnGenerateOffer, ChSignalRegister)

	// Step 3. createoffer
	go func() {
		<-ChOnGenerateOffer //wait
		//generateOffer(pc)
	}()


	// Step 5. getRemoteBoxSdp
	go func() {

		for{
			time.Sleep(time.Second)
			sdp := getRemoteServerSdp()
			if sdp != "" {
				resdp := handleRemoteBoxSdpUtilSuccess(sdp , pc)
				ChStartRegAppSdp <- resdp
			}
			break
		}

	}()

	// Step 6. setAppLocalRemoteSdp
	go func() {
		app_sdp := <-ChStartRegAppSdp //wait
		//time.Sleep(time.Second*5)
		setAppRemoteSdp(app_sdp)
		<- ChDcEnable

		ChAllOk<-1
	}()

	// Step 7. blocked & loop & print status info
	//var endchat bool = false
	//dc = prepareDataChannel(pc, &endchat)
	time.Sleep(5 * time.Second)
	fmt.Printf("====Waiting all ok===\n", )
	<-ChAllOk
	//for !endchat{
	//	msg := "i am client 1\n"
	//
	//	if dc != nil {
	//		fmt.Printf("DataChannel state : %s\n", dc.ReadyState().String())
	//		fmt.Printf("client send data : %s\n", msg)
	//		dc.Send([]byte(msg))
	//	}
	//
	//	time.Sleep(5 * time.Second)
	//}

}

func main(){

	fmt.Println("!!!Start a new session!!!!")
	mainprocess()

	e := echo.New()

	e.POST("/ubeybox/*" , ProxyUbeyboxReq)

	e.Start(":10086")
}



func ProxyUbeyboxReq(c echo.Context) error {

	fmt.Printf("get req :%s\n",c.Request().RequestURI)

	req := protocol.WebRtcReq{}
	reqHeader := make(map[string][]string)


	req.Url = c.Request().RequestURI
	for k , v := range c.Request().Header {
		reqHeader[k] = v
	}

	bHeader , _ := json.Marshal(reqHeader)
	req.Header = string(bHeader)

	req.Method = "POST"
	bBody , _ := ioutil.ReadAll(c.Request().Body)
	req.Body = string(bBody)


	reqData , _ := json.Marshal(req)
	dataStr := string(base64.StdEncoding.EncodeToString(reqData))
	data := []byte(dataStr + "\n")
	//data := protocol.GetProtManagerIns().PackData(req) + "\n"

	l := len(data)
	mid := l

	dc.Send([]byte(data[:mid]))
	fmt.Printf("client send data :%s\n",data[:mid])
	//time.Sleep(time.Second * 2)
	//dc.Send([]byte(data[mid:l]))
	//fmt.Printf("client send data :%s\n",data[mid:l])
	rsp := <- proxy.GetCliManager().ChRsp

	c.Response().Status = rsp.Code

	rspHeader := make(map[string][]string)
	json.Unmarshal([]byte(rsp.Header) , &rspHeader)

	for k , v := range rspHeader {
		for _ , v2 := range v {
			c.Response().Header().Add(k , v2)
		}
	}

	c.Response().Write([]byte(rsp.Body))
	fmt.Printf("get rsp :%+v\n",rsp)
	return c.JSONPretty(200, "" , "")
}


func createpc() *webrtc.PeerConnection {
	fmt.Println("Initbox...")
	fmt.Println("Starting up PeerConnection config...")

	urls := []string{"turn:iamtest.yqtc.co:3478?transport=udp"}
	s := webrtc.IceServer{Urls: urls, Username: "1531542280:guest", Credential: "xAhVJq3B18x2tdaFQUeYc3DcK9k="}  //Credential:"turn.yqtc.top"
	webrtc.NewIceServer()
	config := webrtc.NewConfiguration()
	config.IceServers = append(config.IceServers, s)
	//config.IceTransportPolicy = webrtc.IceTransportPolicyRelay

	pc, err := webrtc.NewPeerConnection(config)
	if nil != err {
		fmt.Println("Failed to create PeerConnection.")
		return pc
	}
	return pc
}

func registerCallback(pc *webrtc.PeerConnection, ChCanGenOffer chan int, ChCanRegisterSdp chan string) {
	// OnNegotiationNeeded is triggered when something important has occurred in
	// the state of PeerConnection (such as creating a new data channel), in which
	// case a new SDP offer must be prepared and sent to the remote peer.
	pc.OnNegotiationNeeded = func() {
		ChCanGenOffer <- 1
	}

	// Once all ICE candidates are prepared, they need to be sent to the remote
	// peer which will attempt reaching the local peer through NATs.
	pc.OnIceComplete = func() {
		fmt.Println("Finished gathering ICE candidates.")
		sdp := pc.LocalDescription().Serialize()
		ChCanRegisterSdp <- sdp
	}

	pc.OnDataChannel = func(channel *webrtc.DataChannel) {
		fmt.Println("Datachannel established by remote... ", channel.Label())
		dc = channel
		datachannlePrepare(channel)
	}
}


func generateAnswer(pc *webrtc.PeerConnection) {
	fmt.Println("Generating answer...")
	answer, err := pc.CreateAnswer() // blocking
	if err != nil {
		fmt.Println(err)
		return
	}
	pc.SetLocalDescription(answer)
	ChAnswerSdpReady <- pc.LocalDescription().Serialize()
}


func getRemoteServerSdp() string {
	fmt.Println(" ---- get sdp connect from host ---- ")

	body := SdpReq{}

	body.Box_id = BOXID
	body.Action = 0

	url := "http://iamtest.yqtc.co/ubbey/sdp/app_get_box_sdp"

	b , _ := json.Marshal(body)

	req , _ := http.NewRequest("POST",url,bytes.NewReader(b))
	req.Header.Add("Content-type","application/json")

	cli := http.Client{}

	tr := &http.Transport{TLSClientConfig:&tls.Config{InsecureSkipVerify:true}}
	cli.Transport = tr

	r , err := cli.Do(req)
	if err != nil {
		fmt.Printf("http err :%s\n",err.Error())
	}
	rspb, _ := ioutil.ReadAll(r.Body)

	rsp := SdpRsp{}
	json.Unmarshal(rspb , &rsp)

	fmt.Printf("get server sdp rsp %+v\n",rsp)
	//if rsp.Box_sdp != "" {
	//	signalReceive(rsp.Box_sdp)
	//}
	return rsp.Box_sdp
}

func setAppRemoteSdp(msg string){
	fmt.Println(" ---- register sdp to host ---- ")

	url := "http://iamtest.yqtc.co/ubbey/sdp/app_register_sdp"

	body := SdpReq{}

	body.Box_id = BOXID
	body.Action = 1
	body.App_sdp = msg

	b , _ := json.Marshal(body)

	req , _ := http.NewRequest("POST",url,bytes.NewReader(b))
	req.Header.Add("Content-type","application/json")

	cli := http.Client{}

	tr := &http.Transport{TLSClientConfig:&tls.Config{InsecureSkipVerify:true}}
	cli.Transport = tr

	r , err := cli.Do(req)
	if err != nil {
		fmt.Printf("http err :%s\n",err.Error())
	}

	rspb, _ := ioutil.ReadAll(r.Body)
	fmt.Printf("local sdp :%s\n",msg)
	fmt.Printf("http rsp :%s, register done \n",rspb)

}

func handleRemoteBoxSdpUtilSuccess(box_sdp string , pc *webrtc.PeerConnection) string {

	data := make(map[string]string)

	json.Unmarshal([]byte(box_sdp) , &data)

	sdp := data["sdp"]

	sdps := strings.Split(sdp," ")

	sessionid := sdps[1]


	setAppLocalRemoteSdp(box_sdp , pc)


	answer := <- ChAnswerSdpReady
	fmt.Printf("answer sdp :%s\n",answer)
	answerData :=  make(map[string]string)

	json.Unmarshal([]byte(answer) , &answerData)

	answerData["myrandsessionid"] = sessionid

	remoteSdp , _ := json.Marshal(answerData)
	return string(remoteSdp)
}

func setAppLocalRemoteSdp(msg string, pc *webrtc.PeerConnection) {
	var parsed map[string]interface{}
	err := json.Unmarshal([]byte(msg), &parsed)
	if nil != err {
		fmt.Println(err, ", try again.")
		return
	}

	// If this is a valid signal and no PeerConnection has been instantiated,
	// start as the "answerer."

	if nil != parsed["sdp"] {
		sdp := webrtc.DeserializeSessionDescription(msg)
		if nil == sdp {
			fmt.Println("Invalid SDP.")
			return
		}

		err = pc.SetRemoteDescription(sdp)
		if nil != err {
			fmt.Println("ERROR", err)
			return
		}
		fmt.Println("SDP " + sdp.Type + " successfully received.")
		if "offer" == sdp.Type {
			go generateAnswer(pc)
		}
	}

	// Allow individual ICE candidate messages, but this won't be necessary if
	// the remote peer also doesn't use trickle ICE.
	if nil != parsed["candidate"] {
		ice := webrtc.DeserializeIceCandidate(msg)
		if nil == ice {
			fmt.Println("Invalid ICE candidate.")
			return
		}
		pc.AddIceCandidate(*ice)
		fmt.Println("ICE candidate successfully received.")
	}
}

func datachannlePrepare(channl *webrtc.DataChannel) *webrtc.DataChannel {
	channl.OnOpen = func() {
		fmt.Println("Data Channel Opened!")
		ChDcEnable <- "done"
		//startChat()
	}
	channl.OnClose = func() {
		fmt.Println("Data Channel closed.")
	}
	channl.OnMessage = func(msg []byte) {
		fmt.Printf("recv msg : %s\n", msg)

		proxy.GetCliManager().PutData(msg)
	}
	return channl
}
