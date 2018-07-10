package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
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
	Box_sdp string `json:"box_sdp"`
}

type SdpRsp struct {
	Err_no  int    `json:"err_no"`
	Err_msg string `json:"err_msg"`
	App_sdp string `json:"app_sdp"`
}

const BOXID = "123"


var ChSignalNewConn = make(chan int, 1)
func mainprocess() {
	var (
		ChOnGenerateOffer = make(chan int, 1)
		ChSignalRegister  = make(chan string, 1)
		ChStartGetAppSdp  = make(chan string, 1)
		ChStartSetBoxSdp  = make(chan string, 1)
		dc *webrtc.DataChannel
		ChAllOk = make(chan int, 1)
	)

	// Step 1. create pc
	pc := createpc()

	// Step 2. register callback
	registerCallback(pc, dc, ChOnGenerateOffer, ChSignalRegister)

	// Step 3. createoffer
	go func() {
		<-ChOnGenerateOffer //wait
		generateOffer(pc)
	}()

	// Step 4. registerBoxSdp
	go func() {
		sdp := <-ChSignalRegister //wait
		registerBoxSdp(sdp)
		ChStartGetAppSdp <- sdp
	}()

	// Step 5. getRemoteAppSdp
	go func() {
		box_sdp := <-ChStartGetAppSdp //wait
		time.Sleep(time.Second * 30)
		app_sdp := getRemoteAppSdpUtilSuccess(box_sdp)
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
	for !endchat{
		msg := "i am server\n"
		fmt.Printf("server send data : %s\n", msg)
		fmt.Printf("DataChannel state : %s\n", dc.ReadyState().String())
		dc.Send([]byte(msg))
		time.Sleep(5 * time.Second)
	}
}
func main(){
	for{
		fmt.Println("!!!Start a new session!!!!")

		go mainprocess()
		<-ChSignalNewConn
	}
}

func createpc() *webrtc.PeerConnection {
	fmt.Println("Initbox...")
	fmt.Println("Starting up PeerConnection config...")
	urls := []string{"turn:iamtest.yqtc.co:3478?transport=udp"}
	s := webrtc.IceServer{Urls: urls, Username: "1531277854:guest", Credential: "3dLgnggMLsyTCOb5CF+jcOznZ8A="} //Credential:"turn.yqtc.top"
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
	url := "http://iamtest.yqtc.co/ubbey/turn/box_sdp"
	body := SdpReq{}

	body.Box_id = BOXID
	body.Action = 0
	body.Box_sdp = msg

	b, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", url, bytes.NewReader(b))
	req.Header.Add("Content-type", "application/json")
	cli := http.Client{}

	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	cli.Transport = tr

	r, err := cli.Do(req)
	if err != nil {
		fmt.Printf("http err :%s\n", err.Error())
	}

	rspb, _ := ioutil.ReadAll(r.Body)

	fmt.Printf("http rsp :%s\n", rspb)

}

func getRemoteAppSdp() (sdp string, boxsid string, err error) {
	fmt.Println(" ---- get sdp connect from host ---- ")

	body := SdpReq{}
	body.Box_id = BOXID
	body.Action = 1

	url := "http://iamtest.yqtc.co/ubbey/turn/box_sdp"

	b, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", url, bytes.NewReader(b))
	req.Header.Add("Content-type", "application/json")

	cli := http.Client{}

	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	cli.Transport = tr

	r, err := cli.Do(req)
	if err != nil {
		fmt.Printf("http err :%s\n", err.Error())
		return "", "", err
	}

	rspb, _ := ioutil.ReadAll(r.Body)
	fmt.Printf("sdp http success :%s\n", rspb)
	rsp := SdpRsp{}
	json.Unmarshal(rspb, &rsp)

	fmt.Printf("get sdp rsp %+v\n", rsp)
	if rsp.App_sdp == "" {
		return "", "", errors.New("rsp.App_sdp is empty")
	}

	var objmap map[string]*json.RawMessage
	err = json.Unmarshal([]byte(rsp.App_sdp), &objmap)
	if err != nil {
		fmt.Printf("Unmarshal App_sdp error:%s\n", err.Error())
		return "", "", err
	} else if _, ok := objmap["myrandsessionid"]; ok && nil == json.Unmarshal(*objmap["myrandsessionid"], &boxsid) {
		sdp = rsp.App_sdp
		return sdp, boxsid, nil
	} else {
		return "", "", errors.New("rsp.App_sdp have no 'myrandsessionid' field")
	}
}

func getRemoteAppSdpUtilSuccess(box_sdp string) (appsdp string) {
	for {
		app_sdp, box_sid, err := getRemoteAppSdp()
		if err == nil && strings.Index(box_sdp, box_sid) >= 0 {
			fmt.Printf("====Success get app_sdp:%s, ===box_sdp:%s, ===box_sid:%s\n",app_sdp, box_sdp, box_sid)
			fmt.Println("====Success get app_sdp, ready to setBoxLocalRemoteSdp")
			appsdp = app_sdp
			break
		} else if err == nil {
			fmt.Printf("====Success get app_sdp:%s, ===box_sdp:%s, ===box_sid:%s\n",app_sdp, box_sdp, box_sid)
			fmt.Println("====Success get app_sdp, but box_sid NOT match, retry....")
			time.Sleep(5 * time.Second)
			continue
		} else {
			fmt.Println("====Failed get app_sdp, retry, error=" + err.Error())
			time.Sleep(5 * time.Second)
			continue
		}
	}
	return
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