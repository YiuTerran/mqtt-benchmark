package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/GaryBoone/GoStats/stats"
)

// Message describes a message
type Message struct {
	Topic     string
	QoS       byte
	Payload   interface{}
	Sent      time.Time
	Delivered time.Time
	Error     bool
}

// RunResults describes results of a single client / run
type RunResults struct {
	ID          int     `json:"id"`
	Successes   int64   `json:"successes"`
	Failures    int64   `json:"failures"`
	RunTime     float64 `json:"run_time"`
	MsgTimeMin  float64 `json:"msg_time_min"`
	MsgTimeMax  float64 `json:"msg_time_max"`
	MsgTimeMean float64 `json:"msg_time_mean"`
	MsgsPerSec  float64 `json:"msgs_per_sec"`
}

// TotalResults describes results of all clients / runs
type TotalResults struct {
	ConnRatio          float64 `json:"conn_ratio"`            //连接成功率
	AvgConnTime        float64 `json:"avg_conn_time"`         //平均连接时间
	ConnPerSec         float64 `json:"conn_per_sec"`          //平均每秒建立连接数
	Ratio              float64 `json:"ratio"`                 //消息发送成功率
	Successes          int64   `json:"successes"`             //总成功消息数
	Failures           int64   `json:"failures"`              //总失败数
	TotalRunTime       float64 `json:"total_run_time"`        //总运行时间
	AvgRunTime         float64 `json:"avg_run_time"`          //平均每个客户端运行时间
	MsgTimeMin         float64 `json:"msg_time_min"`          //最快发送消息延迟
	MsgTimeMax         float64 `json:"msg_time_max"`          //最慢发送消息延迟
	MsgTimeMeanAvg     float64 `json:"msg_time_mean_avg"`     //平均发送消息延迟
	TotalMsgsPerSec    float64 `json:"total_msgs_per_sec"`    //每秒发送消息总数
	AvgMsgsPerSec      float64 `json:"avg_msgs_per_sec"`      //每个客户端每秒消息均值
	ReceiveRatio       float64 `json:"receive_ratio"`         //发送消息成功接收到的数值
	ReceiveMsgPerSec   float64 `json:"receive_msg_per_sec"`   //每秒接收到的消息数
	AvgReceiveMsgDelay float64 `json:"avg_receive_msg_delay"` //接收消息的平均延迟(ms)
}

// JSONResults are used to export results as a JSON document
type JSONResults struct {
	Runs   []*RunResults `json:"runs"`
	Totals *TotalResults `json:"totals"`
}

type ConnResult struct {
	Client *Client
	OK     bool
	ConnAt time.Time
}

func main() {
	var (
		broker   = flag.String("broker", "tcp://localhost:1883", "MQTT broker endpoint as scheme://host:port")
		topic    = flag.String("topic", "/test", "MQTT topic for outgoing messages")
		username = flag.String("username", "", "MQTT client username (empty if auth disabled)")
		connWait = flag.Int("conn-wait", 0, "Connect interval in milliseconds (default 0)")
		password = flag.String("password", "", "MQTT client password (empty if auth disabled)")
		qos      = flag.Int("qos", 1, "QoS for published messages")
		wait     = flag.Int("wait", 60000, "QoS 1 wait timeout in milliseconds")
		count    = flag.Int("count", 100, "Number of messages to send per client")
		clients  = flag.Int("clients", 10, "Number of clients to start")
		payload  = flag.String("payload", "hello world", `Content you want to publish. "${createTs}" in payload will be replace by sending timestamp (in ms), "${randn}" will be replaced by random n-size string.
`)
		payloadFormat = flag.String("payload-format", "jo", `jo(json object), ja(json array), or txt(default jo)`)
		file          = flag.String("file", "", `File path. File with a json array, line example: 
        [{"username": "", "password":"", "clientId":"", "qos":0, "payload":"", "count": 1, "wait": 300, "topic":""}]
        These config overwrite the same key above from command line.`)
		clientPrefix = flag.String("client-prefix", "mqtt-benchmark", "MQTT client id prefix (suffixed with '-<client-num>'")
		clientCert   = flag.String("client-cert", "", "Path to client certificate in PEM format")
		clientKey    = flag.String("client-key", "", "Path to private clientKey in PEM format")
	)

	flag.Parse()
	if *clients < 1 {
		*clients = 1
	}
	if *count < 1 {
		*count = 1
	}
	if *clientCert != "" && *clientKey == "" {
		log.Fatal("Invalid arguments: private clientKey path missing")
	}
	if *clientCert == "" && *clientKey != "" {
		log.Fatalf("Invalid arguments: certificate path missing")
	}
	if *payloadFormat != "ja" && *payloadFormat != "jo" && *payloadFormat != "txt" {
		log.Fatalf("Unknown payload format")
	}
	var tlsConfig *tls.Config
	if *clientCert != "" && *clientKey != "" {
		tlsConfig = generateTLSConfig(*clientCert, *clientKey)
	}
	var (
		config *os.File
		err    error
	)
	if file != nil && *file != "" {
		config, err = os.Open(*file)
		if err != nil {
			log.Fatalf("fail to open %s", *file)
		}
		defer config.Close()
	}
	start := time.Now()
	connResultChn := make(chan ConnResult, *clients)
	//从文件中读取配置，字段不存在的就用通用配置
	if config != nil {
		bs, err := ioutil.ReadAll(config)
		if err != nil {
			log.Fatalf("fail to read json file:%s", err)
		}
		var cs []*Client
		if err = json.Unmarshal(bs, &cs); err != nil {
			log.Fatalf("fail to parse json file:%s", err)
		}
		for i, c := range cs {
			c.BrokerURL = *broker
			if c.BrokerUser == "" {
				c.BrokerUser = *username
			}
			if c.BrokerPass == "" {
				c.BrokerPass = *password
			}
			if c.MsgTopic == "" {
				c.MsgTopic = *topic
			}
			if c.Payload == nil {
				switch *payloadFormat {
				case "txt":
					c.Payload = *payload
				default:
					_ = json.Unmarshal([]byte(*payload), &c.Payload)
				}
			}
			if c.MsgCount == 0 {
				c.MsgCount = *count
			}
			if c.WaitTimeout == 0 {
				c.WaitTimeout = *wait
			}
			if c.ClientID == "" {
				c.ClientID = fmt.Sprintf("%s-%d", *clientPrefix, i)
			}
			c.TLSConfig = tlsConfig
			go c.Conn(connResultChn)
			if *connWait > 0 {
				time.Sleep(time.Duration(*connWait) * time.Millisecond)
			}
		}
	} else {
		//根据通用配置创建文件
		for i := 0; i < *clients; i++ {
			c := &Client{
				ID:          i,
				ClientID:    fmt.Sprintf("%s-%d", *clientPrefix, i),
				BrokerURL:   *broker,
				BrokerUser:  *username + strconv.Itoa(i+2),
				BrokerPass:  *password,
				MsgTopic:    *topic,
				MsgCount:    *count,
				MsgQoS:      byte(*qos),
				WaitTimeout: *wait,
				Payload:     *payload,
				TLSConfig:   tlsConfig,
			}
			go c.Conn(connResultChn)
			if *connWait > 0 {
				time.Sleep(time.Duration(*connWait) * time.Millisecond)
			}
		}
	}
	//等待连接结果
	totals := &TotalResults{}
	pubClients := make([]*Client, 0)
	times := make([]float64, 0)
	var lastConnAt time.Time
	for i := 0; i < *clients; i++ {
		result := <-connResultChn
		if result.OK {
			pubClients = append(pubClients, result.Client)
			if result.ConnAt.After(lastConnAt) {
				lastConnAt = result.ConnAt
			}
			times = append(times, float64(result.ConnAt.Sub(start).Milliseconds()))
		}
	}
	//统计连接相关数据
	totals.ConnRatio = float64(len(pubClients)) / float64(*clients)
	totals.ConnPerSec = float64(len(pubClients)) / (lastConnAt.Sub(start).Seconds())
	totals.AvgConnTime = float64(lastConnAt.Sub(start).Milliseconds()) / float64(len(pubClients))
	fmt.Printf(">>>>>>%v clients start pub...\n", len(pubClients))
	//开始发送消息
	start = time.Now()
	resCh := make(chan *RunResults, len(pubClients))
	for _, c := range pubClients {
		go c.Run(resCh)
	}
	//统计发送结果
	results := make([]*RunResults, 0, len(pubClients))
	for i := 0; i < len(pubClients); i++ {
		r := <-resCh
		results = append(results, r)
	}
	totals.TotalRunTime = time.Now().Sub(start).Seconds()
	calculateTotalResults(results, totals)
	//打印结果
	printResults(results, totals)
}

func calculateTotalResults(results []*RunResults, totals *TotalResults) {
	msgTimeMeans := make([]float64, len(results))
	msgsPerSecs := make([]float64, len(results))
	runTimes := make([]float64, len(results))
	bws := make([]float64, len(results))

	totals.MsgTimeMin = results[0].MsgTimeMin
	for i, res := range results {
		totals.Successes += res.Successes
		totals.Failures += res.Failures

		if res.MsgTimeMin < totals.MsgTimeMin {
			totals.MsgTimeMin = res.MsgTimeMin
		}

		if res.MsgTimeMax > totals.MsgTimeMax {
			totals.MsgTimeMax = res.MsgTimeMax
		}

		msgTimeMeans[i] = res.MsgTimeMean
		msgsPerSecs[i] = res.MsgsPerSec
		runTimes[i] = res.RunTime
		bws[i] = res.MsgsPerSec
	}
	totals.TotalMsgsPerSec = float64(totals.Successes) / totals.TotalRunTime
	totals.Ratio = float64(totals.Successes) / float64(totals.Successes+totals.Failures)
	totals.AvgMsgsPerSec = stats.StatsMean(msgsPerSecs)
	totals.AvgRunTime = stats.StatsMean(runTimes)
	totals.MsgTimeMeanAvg = stats.StatsMean(msgTimeMeans)
}

func printResults(results []*RunResults, totals *TotalResults) {
	fmt.Printf("========= TOTAL (%d) =========\n", len(results))
	fmt.Printf("Connect Ratio:				%.3f\n", totals.ConnRatio)
	fmt.Printf("Average Connect Time (ms):	%.3f\n", totals.AvgConnTime)
	fmt.Printf("Connect Speed(conn/sec)		%.3f\n", totals.ConnPerSec)
	fmt.Printf("Total Ratio:                 %.3f (%d/%d)\n", totals.Ratio, totals.Successes, totals.Successes+totals.Failures)
	fmt.Printf("Total Runtime (sec):         %.3f\n", totals.TotalRunTime)
	fmt.Printf("Average Runtime (sec):       %.3f\n", totals.AvgRunTime)
	fmt.Printf("Msg time min (ms):           %.3f\n", totals.MsgTimeMin)
	fmt.Printf("Msg time max (ms):           %.3f\n", totals.MsgTimeMax)
	fmt.Printf("Msg time mean mean (ms):     %.3f\n", totals.MsgTimeMeanAvg)
	fmt.Printf("Average Bandwidth (msg/sec): %.3f\n", totals.AvgMsgsPerSec)
	fmt.Printf("Total Bandwidth (msg/sec):   %.3f\n", totals.TotalMsgsPerSec)
}

func generateTLSConfig(certFile string, keyFile string) *tls.Config {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		log.Fatalf("Error reading certificate files: %v", err)
	}

	cfg := tls.Config{
		ClientAuth:         tls.NoClientCert,
		ClientCAs:          nil,
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{cert},
	}

	return &cfg
}
