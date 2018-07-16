package proxy

import (
	"ubox.golib/p2p/protocol"
	"bytes"
	"fmt"
	"encoding/base64"
	"encoding/json"

	"reflect"
)

type dcManager struct {
	Buffer 	*bytes.Buffer
	ChRsp   chan protocol.WebRtcRsp
	ChReq 	chan protocol.WebRtcReq
}

var instance *dcManager

func GetCliManager() *dcManager {
	if instance == nil {
		instance = new(dcManager)
	//	buf := make([]byte,1024 * 1024)
		instance.Buffer = new(bytes.Buffer)
		instance.ChRsp = make(chan protocol.WebRtcRsp , 10)
		instance.ChReq = make(chan protocol.WebRtcReq , 10)
	}
	return instance
}

func (pdc *dcManager) PutData(buf []byte) {
	n , err := pdc.Buffer.Write(buf)

	if err != nil {
		fmt.Printf("write buffer error %s\n",err.Error())
	}
	fmt.Printf("write buffer data success n:%d data:%s\n",n,buf[:n])
	pdc.parseData()
}

func (pdc *dcManager) parseData() {
	data , err := pdc.Buffer.ReadString('\n')
	if err != nil {
		pdc.Buffer.Write([]byte(data))
		fmt.Printf("read buffer err :%s\n",err.Error())
		return
	}


	//protocol.GetProtManagerIns().HandleRequest(data , nil)
	bdata , err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		fmt.Printf("decode string err :%s\n",err.Error())
	}
	fmt.Printf("read buffer data success :%s\n",bdata)
	//p := protocol.Protocol{}
	p := protocol.WebRtcReq{}
	json.Unmarshal(bdata , &p)

	fmt.Printf("get req %+v\n",p)

	t := reflect.TypeOf(p)
	if t.Name() == "WebRtcReq" {
		pdc.ChReq <- p
	}
	//if p.Reqname == "WebRtcRsp" {
	//	rp := protocol.WebRtcRsp{}
	//	json.Unmarshal(p.Data , &rp)
	//	pdc.ChRsp <- rp
	//}
	//if p.Reqname == "WebRtcReq" {
	//	rp := protocol.WebRtcReq{}
	//	json.Unmarshal(p.Data , &rp)
	//	pdc.ChReq <- rp
	//}

}




