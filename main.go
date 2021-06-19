package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
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
		broker       = flag.String("broker", "tcp://localhost:1883", "MQTT broker endpoint as scheme://host:port")
		topic        = flag.String("topic", "/test", "MQTT topic for outgoing messages")
		username     = flag.String("username", "", "MQTT client username (empty if auth disabled)")
		password     = flag.String("password", "", "MQTT client password (empty if auth disabled)")
		qos          = flag.Int("qos", 1, "QoS for published messages")
		wait         = flag.Int("wait", 60000, "QoS 1 wait timeout in milliseconds")
		count        = flag.Int("count", 100, "Number of messages to send per client")
		clients      = flag.Int("clients", 10, "Number of clients to start")
		format       = flag.String("format", "text", "Output format: text|json")
		quiet        = flag.Bool("quiet", false, "Suppress logs while running")
		clientPrefix = flag.String("client-prefix", "mqtt-benchmark", "MQTT client id prefix (suffixed with '-<client-num>'")
		clientCert   = flag.String("client-cert", "", "Path to client certificate in PEM format")
		clientKey    = flag.String("client-key", "", "Path to private clientKey in PEM format")
	)

	flag.Parse()
	if *clients < 1 {
		log.Fatalf("Invalid arguments: number of clients should be > 1, given: %v", *clients)
	}

	if *count < 1 {
		log.Fatalf("Invalid arguments: messages count should be > 1, given: %v", *count)
	}

	if *clientCert != "" && *clientKey == "" {
		log.Fatal("Invalid arguments: private clientKey path missing")
	}

	if *clientCert == "" && *clientKey != "" {
		log.Fatalf("Invalid arguments: certificate path missing")
	}

	var tlsConfig *tls.Config
	if *clientCert != "" && *clientKey != "" {
		tlsConfig = generateTLSConfig(*clientCert, *clientKey)
	}
	start := time.Now()
	connResultChn := make(chan ConnResult, *clients)
	totals := &TotalResults{}
	for i := 0; i < *clients; i++ {
		c := &Client{
			ID:          i,
			ClientID:    *clientPrefix,
			BrokerURL:   *broker,
			BrokerUser:  *username + strconv.Itoa(i+2),
			BrokerPass:  *password,
			MsgTopic:    *topic,
			MsgCount:    *count,
			MsgQoS:      byte(*qos),
			Quiet:       *quiet,
			WaitTimeout: time.Duration(*wait) * time.Millisecond,
			TLSConfig:   tlsConfig,
		}
		go c.Conn(connResultChn)
		time.Sleep(time.Duration(rand.Int31n(2)+1) * time.Millisecond)
	}
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
	// collect the results
	results := make([]*RunResults, 0, len(pubClients))
	for i := 0; i < len(pubClients); i++ {
		r := <-resCh
		results = append(results, r)
	}
	totals.TotalRunTime = time.Now().Sub(start).Seconds()
	calculateTotalResults(results, totals)
	// print stats
	printResults(results, totals, *format)
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

func printResults(results []*RunResults, totals *TotalResults, format string) {
	switch format {
	case "json":
		jr := JSONResults{
			Totals: totals,
		}
		data, err := json.Marshal(jr)
		if err != nil {
			log.Fatalf("Error marshalling results: %v", err)
		}
		var out bytes.Buffer
		_ = json.Indent(&out, data, "", "\t")
		fmt.Println(string(out.Bytes()))
	default:
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
	return
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
