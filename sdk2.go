package bililive

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	uuid "github.com/satori/go.uuid"
	"log"
	"time"
)

type authReq struct {
	RoomID   int    `json:"roomid"`
	BuVID    string `json:"buvid"`
	UserID   int64  `json:"uid"`
	ProtoVer int    `json:"protover"`
	Platform string `json:"platform"`
	Type     int    `json:"type"`
	Key      string `json:"key"`
}

type authRsp struct {
	Code int `json:"code"`
}

func NewClient(roomID int) *client {
	return &client{roomID: roomID}
}

type client struct {
	roomID             int // 房间ID（兼容短ID）
	realRoomID         int
	uid                int
	cancel             context.CancelFunc
	hostServerList     []*hostServerList
	currentServerIndex int
	token              string // key
	conn               *websocket.Conn
}

func (c *client) Connect() {
	c.findServer()
	for _, server := range c.hostServerList {
		uri := fmt.Sprintf("wss://%s:%d/sub", server.Host, server.WssPort)
		conn, r, err := websocket.DefaultDialer.Dial(uri, nil)
		if err != nil {
			fmt.Println(err)
			continue
		}
		if r != nil {
			fmt.Println(r.Status)
		}
		c.conn = conn
	}
	c.sendAuth()
}

func (c *client) findServer() error {
	resRoom, err := httpSend(fmt.Sprintf(roomInitURL, c.roomID))
	if err != nil {
		return err
	}
	roomInfo := roomInfoResult{}
	_ = json.Unmarshal(resRoom, &roomInfo)
	if roomInfo.Code != 0 || roomInfo.Data == nil {
		return errors.New("房间不正确")
	}
	c.realRoomID = roomInfo.Data.RoomID
	c.uid = roomInfo.Data.UID
	resDanmuConfig, err := httpSend(fmt.Sprintf(danmuConfigURL, c.realRoomID))
	if err != nil {
		return err
	}
	danmuConfig := danmuServerV2Resp{}
	_ = json.Unmarshal(resDanmuConfig, &danmuConfig)
	if danmuConfig.Code != 0 {
		return errors.New("获取弹幕服务器失败")
	}
	serverList := []*hostServerList{}
	for _, server := range danmuConfig.Data.HostList {
		serverList = append(serverList, &hostServerList{
			Host:    server.Host,
			Port:    server.Port,
			WssPort: server.WssPort,
			WsPort:  server.WsPort,
		})
	}
	c.hostServerList = serverList
	c.token = danmuConfig.Data.Token
	c.currentServerIndex = 0
	return nil
}

func (c *client) sendAuth() {
	authReq, authRsp := &authReq{
		RoomID:   c.realRoomID,
		BuVID:    uuid.NewV4().String(),
		UserID:   int64(c.uid),
		ProtoVer: 3,
		Platform: "web",
		Type:     2,
		Key:      c.token,
	}, &authRsp{}
	payload, err := json.Marshal(authReq)
	if err != nil {
		log.Println(err)
		return
	}
	c.sendData(WS_OP_USER_AUTHENTICATION, payload)
	msg, bytes, err := c.conn.ReadMessage()
	if msg == websocket.BinaryMessage && len(bytes) > int(WS_PACKAGE_HEADER_TOTAL_LENGTH) {
		json.Unmarshal(bytes[WS_PACKAGE_HEADER_TOTAL_LENGTH:], authRsp)
		if authRsp.Code == 0 {
			log.Println("connect success")
			go func() {
				for {
					bytes, _ := base64.StdEncoding.DecodeString("AAAAHwAQAAEAAAACAAAAAVtvYmplY3QgT2JqZWN0XQ==")
					printBytes("up", bytes)
					err := c.conn.WriteMessage(websocket.BinaryMessage, bytes)
					if err != nil {
						log.Println(err)
						time.Sleep(time.Second * 1)
					} else {
						time.Sleep(time.Second * 30)
					}
				}
			}()
			for {
				msg, bytes, err := c.conn.ReadMessage()
				log.Println(msg, err)
				printBytes("down", bytes)
			}

		}
	}
}

func (c *client) sendData(operation int32, payload []byte) {
	b := bytes.NewBuffer([]byte{})
	head := messageHeader{
		Length:          int32(len(payload)) + WS_PACKAGE_HEADER_TOTAL_LENGTH,
		HeaderLength:    int16(WS_PACKAGE_HEADER_TOTAL_LENGTH),
		ProtocolVersion: WS_HEADER_DEFAULT_VERSION,
		Operation:       operation,
		SequenceID:      WS_HEADER_DEFAULT_SEQUENCE,
	}
	err := binary.Write(b, binary.BigEndian, head)
	if err != nil {
		log.Println(err)
	}
	err = binary.Write(b, binary.LittleEndian, payload)
	if err != nil {
		log.Println(err)
	}
	printBytes("up", b.Bytes())
	err = c.conn.WriteMessage(websocket.BinaryMessage, b.Bytes())
	if err != nil {
		log.Println(err)
	}
}
