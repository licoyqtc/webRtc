package main

import (
	"github.com/keroserene/go-webrtc"
	"fmt"
	"crypto/tls"

	"encoding/json"
	"net/http"

	"bytes"
	"io/ioutil"
	"time"
)


var pc *webrtc.PeerConnection
var dc *webrtc.DataChannel
var err error

type SdpReq struct {
	Box_id 	string	`json:"box_id"`
	Action 	int		`json:"action"`
	App_sdp string	`json:"app_sdp"`
}

type SdpRsp struct {
	Err_no 	int	`json:"err_no"`
	Err_msg string	`json:"err_msg"`
	Box_sdp string	`json:"box_sdp"`
}



func generateOffer() {
	fmt.Println("Generating offer...")
	offer, err := pc.CreateOffer() // blocking
	if err != nil {
		fmt.Println(err)
		return
	}
	pc.SetLocalDescription(offer)
}

func generateAnswer() {
	fmt.Println("Generating answer...")
	answer, err := pc.CreateAnswer() // blocking
	if err != nil {
		fmt.Println(err)
		return
	}
	pc.SetLocalDescription(answer)
}

func receiveDescription(sdp *webrtc.SessionDescription) {
	err = pc.SetRemoteDescription(sdp)
	if nil != err {
		fmt.Println("ERROR", err)
		return
	}
	fmt.Println("SDP " + sdp.Type + " successfully received.")
	if "offer" == sdp.Type {
		go generateAnswer()
	}
}

func signalReceive(msg string) {
	var parsed map[string]interface{}
	err = json.Unmarshal([]byte(msg), &parsed)
	if nil != err {
		// fmt.Println(err, ", try again.")
		return
	}


	if nil != parsed["sdp"] {
		sdp := webrtc.DeserializeSessionDescription(msg)
		if nil == sdp {
			fmt.Println("Invalid SDP.")
			return
		}
		receiveDescription(sdp)
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



func getServerSdp() SdpRsp {
	fmt.Println(" ---- get sdp connect from host ---- ")

	body := SdpReq{}

	body.Box_id = "1234"
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
	if rsp.Box_sdp != "" {
		signalReceive(rsp.Box_sdp)
	}
	return rsp
}

func regClientSdp(msg string){
	fmt.Println(" ---- register sdp to host ---- ")

	url := "http://iamtest.yqtc.co/ubbey/turn/app_connect"

	body := SdpReq{}

	body.Box_id = "1234"
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

	fmt.Printf("http rsp :%s, register done \n",rspb)

}

// Manual "copy-paste" signaling channel.
func signalRegister(msg string) {

	fmt.Println(" ---- register sdp to host ---- ")

	url := "http://iamtest.yqtc.co/turn/app_connect"

	body := SdpReq{}

	body.Box_id = "1234"
	body.Action = 0
	body.App_sdp = msg

	b , _ := json.Marshal(body)

	req , _ := http.NewRequest("POST",url,bytes.NewReader(b))

	cli := http.Client{}

	tr := &http.Transport{TLSClientConfig:&tls.Config{InsecureSkipVerify:true}}
	cli.Transport = tr

	r , err := cli.Do(req)
	if err != nil {
		fmt.Printf("http err :%s\n",err.Error())
	}

	rspb, _ := ioutil.ReadAll(r.Body)

	fmt.Printf("http rsp :%s\n",rspb)

}

// Attach callbacks to a newly created data channel.
// In this demo, only one data channel is expected, and is only used for chat.
// But it is possible to send any sort of bytes over a data channel, for many
// more interesting purposes.
func prepareDataChannel(channel *webrtc.DataChannel) {
	channel.OnOpen = func() {
		fmt.Println("Data Channel Opened!")
		//startChat()
	}
	channel.OnClose = func() {
		fmt.Println("Data Channel closed.")
		//endChat()
	}
	channel.OnMessage = func(msg []byte) {
		fmt.Printf("recv msg : %s\n",msg)
	}
}


func main() {


	//config := webrtc.NewConfiguration(
	//	webrtc.OptionIceServer("turn:139.199.180.239:7002"))
	urls := []string{"turn:139.199.180.239:7002"}
	s := webrtc.IceServer{Urls:urls,Username:"admin",Credential:"turn.yqtc.top"}//Credential:"turn.yqtc.top"

	webrtc.NewIceServer()
	config := webrtc.NewConfiguration()
	config.IceServers = append(config.IceServers , s)
	pc, err = webrtc.NewPeerConnection(config)
	if nil != err {
		fmt.Println("Failed to create PeerConnection.")
		return
	}

	// OnNegotiationNeeded is triggered when something important has occurred in
	// the state of PeerConnection (such as creating a new data channel), in which
	// case a new SDP offer must be prepared and sent to the remote peer.
	pc.OnNegotiationNeeded = func() {
		//go generateOffer()
	}
	// Once all ICE candidates are prepared, they need to be sent to the remote
	// peer which will attempt reaching the local peer through NATs.
	pc.OnIceComplete = func() {
		fmt.Println("Finished gathering ICE candidates.")
		fmt.Printf("local sdp :%s\n",pc.LocalDescription().Serialize())
		//sdp := pc.LocalDescription().Serialize()
		//regClientSdp(sdp)
	}
	/*
		pc.OnIceGatheringStateChange = func(state webrtc.IceGatheringState) {
			fmt.Println("Ice Gathering State:", state)
			if webrtc.IceGatheringStateComplete == state {
				// send local description.
			}
		}
	*/
	// A DataChannel is generated through this callback only when the remote peer
	// has initiated the creation of the data channel.
	pc.OnDataChannel = func(channel *webrtc.DataChannel) {
		fmt.Println("Datachannel established by remote... ", channel.Label())
		dc = channel
		prepareDataChannel(channel)
	}

	// Attempting to create the first datachannel triggers ICE.
	fmt.Println("Initializing datachannel....")
	dc, err = pc.CreateDataChannel("test")
	if nil != err {
		fmt.Println("Unexpected failure creating Channel.")
		return
	}

	getServerSdp()


	for {
		sdp := pc.LocalDescription().Serialize()
		if sdp == "null"{
			fmt.Printf("localsdp null , continue\n")
			continue
		}

		regClientSdp(sdp)
		break
	}

	prepareDataChannel(dc)

	for {

		msg := "i am client\n"
		fmt.Printf("server send data : %s\n",msg)
		dc.Send([]byte(msg))
		time.Sleep(time.Second * 4)
	}


}


