package main

import (
	"encoding/json"
	"fmt"
	"github.com/keroserene/go-webrtc"
	"strings"
	"time"
	"ubox.golib/p2p/protocol"
	"webRtc/proxy"
)

const BOXID = "123"

var (
	ChSignalNewConn = make(chan int, 1)
	sdpManager = make(map[string]string)
	ChRemoteAppSdp  = make(chan string, 1)
)

func mainprocess() {
	var (
		ChOnGenerateOffer = make(chan int, 1)
		ChSignalRegister  = make(chan string, 1)
		ChStartGetAppSdp  = make(chan string, 1)
		pc *webrtc.PeerConnection
		ChStartSetBoxSdp  = make(chan string, 1)
		dc *webrtc.DataChannel
		ChAllOk = make(chan int, 1)
	)

	// Step 1. create pc
	pc = createpc()

	// Step 2. register callback
	registerCallback(pc, dc, ChOnGenerateOffer, ChSignalRegister)

	// Step 3. createoffer
	go func() {
		<-ChOnGenerateOffer //wait
		generateOffer(pc)

		localSdp := pc.LocalDescription().Serialize()
		session := getSdpSession(localSdp)
		sdpManager[session] = localSdp
	}()

	// Step 4. registerBoxSdp
	go func() {
		sdp := <-ChSignalRegister //wait
		registerBoxSdp(sdp)
		ChStartGetAppSdp <- sdp
	}()

	// Step 5. getRemoteAppSdp
	go func() {
		<-ChStartGetAppSdp //wait
		app_sdp := <- ChRemoteAppSdp
		ChStartSetBoxSdp <- app_sdp
	}()

	// Step 6. setBoxLocalRemoteSdp
	go func() {
		app_sdp := <-ChStartSetBoxSdp //wait
		setBoxLocalRemoteSdp(app_sdp, pc)
		ChAllOk<-1
	}()

	// Step 7. blocked & loop & print status info
	var endchat bool = false
	dc = prepareDataChannel(pc, &endchat , dc)
	time.Sleep(5 * time.Second)
	fmt.Printf("====Waiting all ok===\n", )
	<-ChAllOk
	ChSignalNewConn <- 1

	//handleDcReq(dc)
	//for !endchat{
	//	msg := "i am server\n"
	//	fmt.Printf("server send data : %s\n", msg)
	//	fmt.Printf("DataChannel state : %s\n", dc.ReadyState().String())
	//	dc.Send([]byte(msg))
	//	time.Sleep(5 * time.Second)
	//}
}

func main(){

	go protocol.TcpConnect("iamtest.yqtc.co:7005")

	//protocol.GetProtManagerIns().SetFuncHandler(protocol.ReqRegisterSdp{} , proxy.HandleRegisterSdpReq)
	//protocol.GetProtManagerIns().SetFuncHandler(protocol.PushAppSdp{} , proxy.HandleRemoteAppSdp)
	//
	//fmt.Println("!!!Start a new session!!!!")
	//go mainprocess()
	//for{
	//	<-ChSignalNewConn
	//	fmt.Printf("sdp conn stablish succecss!!!\n")
	//}

	go proxy.NewWebRtc().StartUp()

	for {
		time.Sleep(time.Second)
	}
}

//func handleDcReq(dc *webrtc.DataChannel) {
//	for {
//		req := <- proxy.GetCliManager().ChReq
//		fmt.Printf("handleDcReq get req %+v\n",req)
//
//		reader := bytes.NewReader([]byte(req.Body))
//		url := "http://localhost:37867" + req.Url
//		request , _ := http.NewRequest(req.Method , url , reader)
//		for k , v := range req.Header {
//			request.Header[k] = v
//		}
//
//		client := http.Client{}
//
//		response , err := client.Do(request)
//		if err != nil {
//			fmt.Printf("http req err :%s\n",err.Error())
//			return
//		}
//
//		rsp := protocol.WebRtcRsp{}
//		rsp.Header = make(map[string][]string)
//
//		rsp.Code = response.StatusCode
//		for k , v := range response.Header {
//			rsp.Header[k] = v
//		}
//		rsp.Body , _ = ioutil.ReadAll(response.Body)
//
//		rsdata := protocol.GetProtManagerIns().PackData(rsp) + "\n"
//		dc.Send([]byte(rsdata))
//		fmt.Printf("handleDcReq send rsp :%s\n",rsdata)
//	}
//}

func createpc() *webrtc.PeerConnection {
	fmt.Println("Initbox...")
	fmt.Println("Starting up PeerConnection config...")
	urls := []string{"turn:iamtest.yqtc.co:3478?transport=udp"}
	s := webrtc.IceServer{Urls: urls, Username: "1531542280:guest", Credential: "xAhVJq3B18x2tdaFQUeYc3DcK9k="} //Credential:"turn.yqtc.top"
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

func registerCallback(pc *webrtc.PeerConnection, dc *webrtc.DataChannel, ChCanGenOffer chan int, ChCanRegisterSdp chan string) {
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

func generateOffer(pc *webrtc.PeerConnection) {
	fmt.Println("Generating offer...")
	offer, err := pc.CreateOffer() // blocking
	if err != nil {
		fmt.Println(err)
		return
	}

	pc.SetLocalDescription(offer)
}

func registerBoxSdp(msg string) {
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

func handleRegisterSdpReq(context protocol.Context) {
	fmt.Printf("handleRegisterSdpReq get data %+v\n",context.Data)
	req := protocol.ReqRegisterSdp{}
	json.Unmarshal(context.Data , &req)

	go mainprocess()
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


func setBoxLocalRemoteSdp(msg string, pc *webrtc.PeerConnection) {
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

		err = pc.SetRemoteDescription(sdp)
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
		pc.AddIceCandidate(*ice)
		fmt.Println("ICE candidate successfully received.")
	}
	fmt.Println("\nNormal exit setBoxLocalRemoteSdp")
}

func prepareDataChannel(pc *webrtc.PeerConnection, endchat *bool, datachannl *webrtc.DataChannel) (dc *webrtc.DataChannel) {
	// Attempting to create the first datachannel triggers ICE.
	fmt.Println("prepareDataChannel datachannel....")
	datachannl, err := pc.CreateDataChannel("test")
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
		*endchat = true
	}
	datachannl.OnMessage = func(msg []byte) {
		fmt.Printf("recv msg : %s\n", msg)
		proxy.GetCliManager().PutData(msg)
	}
	return datachannl
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

func getSdpSession(sdp string) string {
	data := make(map[string]string)

	json.Unmarshal([]byte(sdp) , &data)

	s := data["sdp"]

	sdps := strings.Split(s," ")

	return sdps[1]
}