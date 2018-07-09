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
			time.Sleep(time.Second*5)
			sdp := getRemoteServerSdp()
			if sdp != "" {
				resdp := handleRemoteBoxSdpUtilSuccess(sdp , pc)
				ChStartRegAppSdp <- resdp
			}

		}


	}()

	// Step 6. setAppLocalRemoteSdp
	go func() {
		app_sdp := <-ChStartRegAppSdp //wait

		setAppRemoteSdp(app_sdp)
		ChAllOk<-1
	}()

	// Step 7. blocked & loop & print status info
	var endchat bool = false
	//dc = prepareDataChannel(pc, &endchat)
	time.Sleep(5 * time.Second)
	fmt.Printf("====Waiting all ok===\n", )
	<-ChAllOk
	for !endchat{
		msg := "i am client\n"

		if dc != nil {
			fmt.Printf("DataChannel state : %s\n", dc.ReadyState().String())
			fmt.Printf("client send data : %s\n", msg)
			dc.Send([]byte(msg))
		}

		time.Sleep(5 * time.Second)
	}
}
func main(){
	for{
		fmt.Println("!!!Start a new session!!!!")
		mainprocess()
	}
}

func createpc() *webrtc.PeerConnection {
	fmt.Println("Initbox...")
	fmt.Println("Starting up PeerConnection config...")
	urls := []string{"turn:139.199.180.239:3478", "stun:139.199.180.239:3478"}
	s := webrtc.IceServer{Urls: urls, Username: "admin", Credential: "admin"} //Credential:"turn.yqtc.top"
	webrtc.NewIceServer()
	config := webrtc.NewConfiguration()
	config.IceServers = append(config.IceServers, s)
	config.IceTransportPolicy = webrtc.IceTransportPolicyRelay

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

	url := "http://iamtest.yqtc.co/ubbey/turn/app_connect"

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

	url := "http://iamtest.yqtc.co/ubbey/turn/app_connect"

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

func prepareDataChannel(pc *webrtc.PeerConnection, endchat *bool) (dc *webrtc.DataChannel) {
	// Attempting to create the first datachannel triggers ICE.
	fmt.Println("prepareDataChannel datachannel....")
	dc, err := pc.CreateDataChannel("test")
	if nil != err {
		fmt.Println("Unexpected failure creating Channel.")
		return
	}

	dc.OnOpen = func() {
		fmt.Println("Data Channel Opened!")
		//startChat()
	}
	dc.OnClose = func() {
		fmt.Println("Data Channel closed.")
		*endchat = true
	}
	dc.OnMessage = func(msg []byte) {
		fmt.Printf("recv msg : %s\n", msg)
	}
	return dc
}

func datachannlePrepare(channl *webrtc.DataChannel) *webrtc.DataChannel {
	channl.OnOpen = func() {
		fmt.Println("Data Channel Opened!")
		//startChat()
	}
	channl.OnClose = func() {
		fmt.Println("Data Channel closed.")
	}
	channl.OnMessage = func(msg []byte) {
		fmt.Printf("recv msg : %s\n", msg)
	}
	return channl
}
