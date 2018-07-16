package proxy

import (
	"github.com/keroserene/go-webrtc"
	"fmt"
	"strings"
	"encoding/json"
	"bytes"
	"net/http"
	"ubox.golib/p2p/protocol"
	"io/ioutil"
)

var (
	sdpManager = make(map[string]string)
	ChRemoteAppSdp  = make(chan string, 10)
)

const BOXID = "123"

type webRtc struct {
	chOnGenerateOffer chan int
	chSignalRegister  chan string
	chStartGetAppSdp  chan string
	pc 				  *webrtc.PeerConnection
	chStartSetBoxSdp  chan string
	dc 			      *webrtc.DataChannel
	chAllOk 		  chan int
}

func NewWebRtc() *webRtc {
	ins := &webRtc{}
	ins.init()
	return ins
}

func (wr *webRtc) StartUp() {


	// Step 1. create pc
	wr.createConn()

	// Step 2. register callback
	wr.registerCallback()

	// Step 3. createoffer
	go func() {
		<- wr.chOnGenerateOffer //wait

		wr.generateOffer()

		localSdp := wr.pc.LocalDescription().Serialize()
		session := getSdpSession(localSdp)
		sdpManager[session] = localSdp

		fmt.Printf("session :%s sdp :%s\n",session , localSdp)
	}()

	// Step 4. registerBoxSdp
	go func() {
		sdp := <- wr.chSignalRegister //wait
		wr.registerBoxSdp(sdp)
		wr.chStartGetAppSdp <- sdp
	}()

	// Step 5. getRemoteAppSdp
	go func() {
		<- wr.chStartGetAppSdp //wait
		app_sdp := <- ChRemoteAppSdp
		//fmt.Printf("remote sdp %s\n",app_sdp)
		wr.chStartSetBoxSdp <- app_sdp
	}()

	// Step 6. setBoxLocalRemoteSdp
	go func() {
		app_sdp := <- wr.chStartSetBoxSdp //wait
		wr.setBoxLocalRemoteSdp(app_sdp)
		wr.chAllOk <-1
	}()

	// Step 7. blocked & loop & print status info

	wr.prepareDataChannel()

	fmt.Printf("====Waiting all ok===\n", )
	<- wr.chAllOk


	fmt.Printf("====main loop===\n", )
	wr.mainLoop()

}

func (wr *webRtc) mainLoop(){
	for {

		req := <- GetCliManager().ChReq
		fmt.Printf("mainLoop get req %+v\n",req)

		retReq , _ := json.Marshal(req)
		wr.dc.Send(retReq)

		reader := bytes.NewReader([]byte(req.Body))
		url := "http://192.168.0.36:37867" + req.Url
		request , _ := http.NewRequest(req.Method , url , reader)

		reqHeader := make(map[string][]string)
		json.Unmarshal([]byte(req.Header) , &reqHeader)
		for k , v := range reqHeader {
			request.Header[k] = v
		}

		fmt.Printf("do http req , url :%s header :%+v body :%s\n",url,reqHeader,req.Body)
		client := http.Client{}
		response , err := client.Do(request)
		if err != nil {
			fmt.Printf("http req err :%s\n",err.Error())
			return
		}

		rsp := protocol.WebRtcRsp{}

		rspHeader := make(map[string][]string)


		rsp.Code = response.StatusCode
		for k , v := range response.Header {
			rspHeader[k] = v
		}
		rspHeaderStr , _ := json.Marshal(rspHeader)
		rsp.Header = string(rspHeaderStr)

		body , err := ioutil.ReadAll(response.Body)
		if err != nil {
			fmt.Printf("read http rsp err :%s\n",err.Error())
			return
		}

		rsp.Body = string(body)
		rsdata , _ := json.Marshal(rsp)

		fmt.Printf("data channel status :%s\n",wr.dc.ReadyState().String())
		wr.dc.Send([]byte(rsdata))

		fmt.Printf("mainLoop send rsp :%s\n",rsdata)
	}
}


func (wr *webRtc) init() {
	wr.chOnGenerateOffer = make(chan int, 1)
	wr.chSignalRegister  = make(chan string, 1)
	wr.chStartGetAppSdp  = make(chan string, 1)
	wr.chStartSetBoxSdp  = make(chan string, 1)
	wr.chAllOk = make(chan int, 1)

	protocol.GetProtManagerIns().SetFuncHandler(protocol.ReqRegisterSdp{} , handleRegisterSdpReq)
	protocol.GetProtManagerIns().SetFuncHandler(protocol.PushAppSdp{} , handleRemoteAppSdp)

}

func (wr *webRtc) createConn() {
	fmt.Println("Starting up PeerConnection config...")
	urls := []string{"turn:iamtest.yqtc.co:3478?transport=udp"}
	s := webrtc.IceServer{Urls: urls, Username: "1531542280:guest", Credential: "xAhVJq3B18x2tdaFQUeYc3DcK9k="} //Credential:"turn.yqtc.top"
	webrtc.NewIceServer()
	config := webrtc.NewConfiguration()
	config.IceServers = append(config.IceServers, s)
	//config.IceTransportPolicy = webrtc.IceTransportPolicyRelay

	pc, err := webrtc.NewPeerConnection(config)

	wr.pc = pc
	if nil != err {
		fmt.Println("Failed to create PeerConnection.")
		return
	}
	return
}

func (wr *webRtc) registerCallback(){
	// OnNegotiationNeeded is triggered when something important has occurred in
	// the state of PeerConnection (such as creating a new data channel), in which
	// case a new SDP offer must be prepared and sent to the remote peer.
	wr.pc.OnNegotiationNeeded = func() {
		wr.chOnGenerateOffer <- 1
	}

	// Once all ICE candidates are prepared, they need to be sent to the remote
	// peer which will attempt reaching the local peer through NATs.
	wr.pc.OnIceComplete = func() {
		fmt.Println("Finished gathering ICE candidates.")
		sdp := wr.pc.LocalDescription().Serialize()
		wr.chSignalRegister <- sdp
	}

	wr.pc.OnDataChannel = func(channel *webrtc.DataChannel) {
		fmt.Println("Datachannel established by remote... ", channel.Label())
		wr.dc = channel
		wr.datachannlePrepare()
	}
}

func (wr *webRtc) prepareDataChannel() {
	// Attempting to create the first datachannel triggers ICE.
	fmt.Println("prepareDataChannel datachannel....")
	datachannl, err := wr.pc.CreateDataChannel("test")
	if nil != err {
		fmt.Println("Unexpected failure creating Channel.")
		return
	}

	datachannl.OnOpen = func() {
		fmt.Println("Data Channel Opened!")
		//startChat()
	}
	datachannl.OnClose = func() {
		fmt.Println("Data Channel closed.")

	}
	datachannl.OnMessage = func(msg []byte) {
		fmt.Printf("recv msg : %s\n", msg)
		GetCliManager().PutData(msg)
	}

	wr.dc = datachannl
}

func (wr *webRtc) datachannlePrepare() {
	wr.dc.OnOpen = func() {
		fmt.Println("Data Channel Opened!")
		//startChat()
	}
	wr.dc.OnClose = func() {
		fmt.Println("Data Channel closed.")
	}
	wr.dc.OnMessage = func(msg []byte) {
		fmt.Printf("recv msg : %s\n", msg)
	}

}

func (wr *webRtc) generateOffer() {
	fmt.Println("Generating offer...")
	offer, err := wr.pc.CreateOffer() // blocking
	if err != nil {
		fmt.Println(err)
		return
	}

	wr.pc.SetLocalDescription(offer)
}

func (wr *webRtc) setBoxLocalRemoteSdp(msg string) {
	var parsed map[string]interface{}
	err := json.Unmarshal([]byte(msg), &parsed)
	if nil != err {
		fmt.Println(err, ", try again.")
		fmt.Println("input msg=" + msg)
		return
	}

	if nil != parsed["sdp"] {
		sdp := webrtc.DeserializeSessionDescription(msg)
		if nil == sdp {
			fmt.Println("Invalid SDP.")
			return
		}

		err = wr.pc.SetRemoteDescription(sdp)
		if nil != err {
			fmt.Println("ERROR", err)
			return
		}
		fmt.Println("SDP " + sdp.Type + " successfully received.")
	}



	// Allow individual ICE candidate messages, but this won't be necessary if
	// the remote peer also doesn't use trickle ICE.
	if nil != parsed["candidate"] {
		ice := webrtc.DeserializeIceCandidate(msg)
		if nil == ice {
			fmt.Println("Invalid ICE candidate.")
			return
		}
		wr.pc.AddIceCandidate(*ice)
		fmt.Println("ICE candidate successfully received.")
	}
	fmt.Println("\nNormal exit setBoxLocalRemoteSdp")
}

func (wr *webRtc) registerBoxSdp(msg string) {
	fmt.Println(" ---- register sdp to host ---- ")

	req := protocol.RegisterSdp{}
	req.BoxId = BOXID
	req.Sdp = msg

	err := protocol.GetTcpConn().HandleWrite(req)
	if err != nil {
		fmt.Printf("registerBoxSdp register failed , err :%s\n",err.Error())
	} else {
		fmt.Printf("registerBoxSdp register success ...\n")
	}
}

func getSdpSession(sdp string) string {
	data := make(map[string]string)

	json.Unmarshal([]byte(sdp) , &data)

	s := data["sdp"]

	sdps := strings.Split(s," ")

	return sdps[1]
}


// webRtc Handler...

func handleRegisterSdpReq(context protocol.Context) {
	fmt.Printf("handleRegisterSdpReq get data %+v\n",context.Data)
	req := protocol.ReqRegisterSdp{}
	json.Unmarshal(context.Data , &req)

	go NewWebRtc().StartUp()
}

func handleRemoteAppSdp(context protocol.Context)  {
	fmt.Println(" ---- get sdp connect from host ---- ")
	fmt.Printf("handleRemoteAppSdp get data %s\n",context.Data)

	//handle app sdp push req
	req := protocol.PushAppSdp{}
	json.Unmarshal(context.Data , &req)

	//get session from sdp
	sdpPack := map[string]string{}

	json.Unmarshal([]byte(req.AppSdp), &sdpPack)

	sesssion , ok := sdpPack["myrandsessionid"]
	boxSdp := sdpManager[sesssion]

	//compare session whether macth
	rsp := protocol.PushRes{
		ErrNo: 0,
		ErrMsg: "success",
	}
	rsp.RequestId = req.RequestId
	fmt.Printf("app box sdp match , app :%s box%s\n",req.AppSdp,boxSdp)
	if ok && strings.Index(boxSdp , sesssion) >= 0 {
		ChRemoteAppSdp <- req.AppSdp
		fmt.Printf("app sdp match , start to set remote sdp...\n")
	} else {
		fmt.Printf("local sdp :%s remote sdp :%s\n",boxSdp , req.AppSdp)
		rsp.ErrNo = 1001
		rsp.ErrMsg = "app sdp not match box sdp..."
		fmt.Printf("app sdp not match box sdp...\n")
	}

	//send push sdp resp to server
	err := context.Conn.HandleWrite(rsp)
	if err != nil {
		fmt.Printf("handleRemoteAppSdp send rsp err :%s\n",err.Error())
	} else {
		fmt.Printf("handleRemoteAppSdp send rsp success :%+v\n",rsp)
	}
}

