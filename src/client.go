package main

import (
	"crypto/tls"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/GaryBoone/GoStats/stats"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// Client implements an MQTT client running benchmark test
type Client struct {
	ID          int
	ClientID    string `json:"clientId"`
	BrokerURL   string
	BrokerUser  string `json:"username"`
	BrokerPass  string `json:"password"`
	MsgTopic    string `json:"topic"`
	Payload     string `json:"payload"`
	MsgCount    int    `json:"count"`
	MsgQoS      byte   `json:"qos"`
	WaitTimeout int    `json:"wait"`
	TLSConfig   *tls.Config

	client mqtt.Client
}

//Conn 将建立连接单独剥离出来，计算连接数均值
func (c *Client) Conn(result chan ConnResult) {
	opts := mqtt.NewClientOptions().
		SetProtocolVersion(4).
		AddBroker(c.BrokerURL).
		SetClientID(c.ClientID).
		SetCleanSession(true).
		SetAutoReconnect(false)
	if c.BrokerUser != "" || c.BrokerPass != "" {
		opts.SetUsername(c.BrokerUser)
		opts.SetPassword(c.BrokerPass)
	}
	if c.TLSConfig != nil {
		opts.SetTLSConfig(c.TLSConfig)
	}
	client := mqtt.NewClient(opts)
	token := client.Connect()
	ok := token.Wait() && token.Error() == nil
	r := ConnResult{
		Client: c,
		OK:     ok,
	}
	if ok {
		c.client = client
		r.ConnAt = time.Now()
	} else {
		fmt.Printf("client %v fail to conn\n", c.ID)
	}
	result <- r
}

// Run runs benchmark tests and writes results in the provided channel
func (c *Client) Run(res chan *RunResults) {
	newMsgs := make(chan *Message)
	pubMsgs := make(chan *Message)
	runResults := new(RunResults)

	started := time.Now()
	// start generator
	go c.genMessages(newMsgs)
	// start publisher
	go c.pubMessages(newMsgs, pubMsgs)

	runResults.ID = c.ID
	var times []float64
	var count int
	for m := range pubMsgs {
		count++
		if m.Error {
			runResults.Failures++
		} else {
			runResults.Successes++
			f := m.Delivered.Sub(m.Sent).Seconds() * 1000
			times = append(times, f)
		}
		//完成了，统计
		if count == c.MsgCount {
			duration := time.Now().Sub(started)
			runResults.MsgTimeMin = stats.StatsMin(times)
			runResults.MsgTimeMax = stats.StatsMax(times)
			runResults.MsgTimeMean = stats.StatsMean(times)
			runResults.RunTime = duration.Seconds()
			runResults.MsgsPerSec = float64(runResults.Successes) / duration.Seconds()
			res <- runResults
			c.client.Disconnect(100)
			return
		}
	}
}

var letters = []rune("0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randSeq(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func (c *Client) fillPayload() []byte {
	if !strings.Contains(c.Payload, "${") {
		return []byte(c.Payload)
	}
	var resp string
	//替换时间
	if strings.Contains(c.Payload, "${createTs}") {
		resp = strings.ReplaceAll(c.Payload, "${createTs}", fmt.Sprint(time.Now().UnixNano()/1e6))
	}
	//随机字符串
	for i := strings.Index(resp, "${rand"); i >= 0; {
		j := i + 6
		count := make([]byte, 0)
		for ; j < len(resp) && resp[j] != '}'; j++ {
			count = append(count, resp[j])
		}
		if c, e := strconv.Atoi(string(count)); e != nil {
			fmt.Printf("fail to gen rand string, count is %s", string(count))
		} else {
			resp = strings.ReplaceAll(resp, "${rand"+string(count)+"}", randSeq(c))
		}
	}
	return []byte(resp)
}

func (c *Client) genMessages(ch chan *Message) {
	for i := 0; i < c.MsgCount; i++ {
		ch <- &Message{
			Topic:   c.MsgTopic,
			QoS:     c.MsgQoS,
			Payload: c.fillPayload(),
		}
	}
	//nil表示生成完毕
	ch <- nil
}

func waitResult(token mqtt.Token, m *Message, out chan *Message) {
	ok := token.Wait()
	if !ok || token.Error() != nil {
		m.Error = true
	} else {
		m.Delivered = time.Now()
		m.Error = false
	}
	out <- m
}

func (c *Client) pubMessages(in, out chan *Message) {
	for m := range in {
		if m == nil {
			return
		}
		m.Sent = time.Now()
		token := c.client.Publish(m.Topic, m.QoS, false, m.Payload)
		go waitResult(token, m, out)
		time.Sleep(time.Duration(c.WaitTimeout) * time.Millisecond)
	}
}
