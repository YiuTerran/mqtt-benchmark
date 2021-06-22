MQTT benchmarking tool
=========

A simple MQTT (broker) benchmarking tool.

Installation:

```sh
go get github.com/krylovsk/mqtt-benchmark
```

The tool supports multiple concurrent clients, configurable message size, etc:

```sh
$ ./mqtt-benchmark -h
Usage of ./mqtt-benchmark:
  -broker string
    	MQTT broker endpoint as scheme://host:port (default "tcp://localhost:1883")
  -client-cert string
    	Path to client certificate in PEM format
  -client-key string
    	Path to private clientKey in PEM format
  -clients int
    	Number of clients to start (default 10)
  -count int
    	Number of messages to send per client (default 100)
  -conn-wait
        Connect interval in milliseconds (default 0)
  -username string
    	MQTT client username (empty if auth disabled)
  -password string
    	MQTT client password (empty if auth disabled)
  -client-prefix string
    	MQTT client id prefix (suffixed with '-<client-num>' (default "mqtt-benchmark")
  -payload string
        Content you want to publish. "${createTs}" in payload will be replace by sending timestamp (in ms), "${randn}" will be replaced by random n-size string.
  -qos int
    	QoS for published messages (default 1)
  -topic string
    	MQTT topic for outgoing messages (default "/test")
  -wait int
    	Publish interval in milliseconds (default 60000)
  -file string
        File path. File with a json array, line example: 
        [{"username": "", "password":"", "clientId":"", "qos":0, "payload":"", "count": 1, "wait": 300, "topic":""}]
        These config overwrite the same key above from command line.
```

Example use and output:

```sh
> mqtt-benchmark --broker tcp://broker.local:1883 -count 100 -clients 100 -qos 2 -payload "hello, world"
....

======= CLIENT 27 =======
Ratio:               1 (100/100)
Runtime (s):         16.396
Msg time min (ms):   9.466
Msg time max (ms):   1880.769
Msg time mean (ms):  150.193
Msg time std (ms):   201.884
Bandwidth (msg/sec): 6.099

========= TOTAL (100) =========
Total Ratio:                 1 (10000/10000)
Total Runime (sec):          16.398
Average Runtime (sec):       15.514
Msg time min (ms):           7.766
Msg time max (ms):           2034.076
Msg time mean mean (ms):     140.751
Msg time mean std (ms):      13.695
Average Bandwidth (msg/sec): 6.761
Total Bandwidth (msg/sec):   676.112
```

Similarly, in JSON:

```sh
> mqtt-benchmark -broker tcp://broker.local:1883 -f
```
Content in `config.txt`:

```json
[
  {"username": "1", "password":"123456", "clientId":"client-1", "qos":0, "payload":{"createTs": "${createTs}", "data": "123456"}, "count": 1000, "wait": 300},
  {"username": "1", "password":"123456", "clientId":"client-2", "qos":1, "payload":{"createTs": "${createTs}", "data": "${rand1024}"}, "count": 1000, "wait": 300}
]
```