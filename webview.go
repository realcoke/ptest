package ptest

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{}

func serveHome(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(w, homehtml)
	// http.ServeFile(w, r, "home.html")
}

type WebViewer struct {
	InputChan   chan StatReport
	Addr        string
	srv         *http.Server
	data        []StatReport
	outputChans []chan StatReport
	mutex       *sync.Mutex
}

func NewWebViewer(inputChan chan StatReport, addr string) *WebViewer {
	wv := &WebViewer{
		InputChan:   inputChan,
		Addr:        addr,
		data:        make([]StatReport, 0),
		outputChans: make([]chan StatReport, 0),
		mutex:       &sync.Mutex{},
	}
	wv.start()
	return wv
}
func (wv *WebViewer) GenWebSocketHandler() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		outputChan := make(chan StatReport, 100)
		wv.addOutputChan(outputChan)
		defer wv.removeOutputChan(outputChan)

		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("upgrade:", err)
			return
		}
		defer ws.Close()

		for {
			sr := <-outputChan

			b, err := json.Marshal(sr)
			if err != nil {
				log.Println("marshal:", err)
				break
			}

			err = ws.WriteMessage(websocket.TextMessage, b)
			if err != nil {
				log.Println("write:", err)
				break
			}
		}
		log.Println("wv end")
	}

}

func (wv *WebViewer) addOutputChan(c chan StatReport) {
	wv.mutex.Lock()
	defer wv.mutex.Unlock()
	wv.outputChans = append(wv.outputChans, c)
	go func() {
		for _, d := range wv.data {
			c <- d
		}
	}()
}

func (wv *WebViewer) removeOutputChan(c chan StatReport) {
	wv.mutex.Lock()
	defer wv.mutex.Unlock()

	last := len(wv.outputChans) - 1
	for i := range wv.outputChans {
		if wv.outputChans[i] == c {
			wv.outputChans[i], wv.outputChans[last] = wv.outputChans[last], wv.outputChans[i]
			wv.outputChans = wv.outputChans[:last]
			close(c)
			return
		}
	}
}

func (wv *WebViewer) start() {
	r := mux.NewRouter()
	r.HandleFunc("/", serveHome)
	r.HandleFunc("/ws", wv.GenWebSocketHandler())

	wv.srv = &http.Server{
		Addr:         wv.Addr,
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      r,
	}

	go func() {
		if err := wv.srv.ListenAndServe(); err != nil {
			log.Println(err)
		}
	}()

	go func() {
		for {
			statReport, more := <-wv.InputChan
			if !more {
				fmt.Println("webview:no more input")

				wv.mutex.Lock()
				for _, c := range wv.outputChans {
					close(c)
				}
				wv.outputChans = nil
				wv.mutex.Unlock()

				wv.srv.Close()
				return
			}

			wv.mutex.Lock()
			wv.data = append(wv.data, statReport)
			for _, outputChan := range wv.outputChans {
				outputChan <- statReport
			}
			wv.mutex.Unlock()
		}
	}()
}

var homehtml = `
<!doctype html>
<html>

<head>
	<title>Performance test</title>
    <script src="https://www.chartjs.org/dist/2.8.0/Chart.min.js"></script>
	<style>
	canvas{
		-moz-user-select: none;
		-webkit-user-select: none;
		-ms-user-select: none;
	}
	</style>
</head>

<body>
    <div style="width:90%;">
    <table style="width:100%;">
        <tr>
            <td style="width:50%;">
                <canvas id="TPS"></canvas>
            </td>
            <td style="width:50%;">
                <canvas id="responseTime"></canvas>
            </td>
        </tr>
        <tr>
            <td style="width:50%;">
                <canvas id="errorTPS"></canvas>
            </td>
            <td style="width:50%;">
                <canvas id="errorResponseTime"></canvas>
            </td>
        </tr>
        <tr>
            <td style="width:50%;">
                <canvas id="errorRate"></canvas>
            </td>
            <td style="width:50%;">
            </td>
        </tr>
    </table>
</div>
	<br>
    <div id="log"></div>
<script>
var log = document.getElementById("log");

function appendLog(text) {
    var item = document.createElement("div");
    item.innerText = text;

    var doScroll = log.scrollTop > log.scrollHeight - log.clientHeight - 1;
    log.appendChild(item);
    if (doScroll) {
        log.scrollTop = log.scrollHeight - log.clientHeight;
    }
}

var chartColors = {
	red: 'rgb(255, 99, 132)',
	orange: 'rgb(255, 159, 64)',
	yellow: 'rgb(255, 205, 86)',
	green: 'rgb(75, 192, 192)',
	blue: 'rgb(54, 162, 235)',
	purple: 'rgb(153, 102, 255)',
	grey: 'rgb(201, 203, 207)'
};

var configTPS = {
    type: 'line',
    data: {
        labels: [],
        datasets: [{
            label: 'TPS',
            backgroundColor: chartColors.red,
            borderColor: chartColors.red,
            data: [],
            fill: false,
        }]
    },
    options: {
        responsive: true,
        title: {
            display: true,
            text: 'Success-TPS'
        },
        tooltips: {
            mode: 'index',
            intersect: false,
        },
        hover: {
            mode: 'nearest',
            intersect: true
        },
        scales: {
            xAxes: [{
                display: true,
                scaleLabel: {
                    display: true,
                    labelString: 'Time'
                }
            }],
            yAxes: [{
                display: true,
                scaleLabel: {
                    display: true,
                    labelString: 'TPS'
                }
            }]
        }
    }
};


var configResponseTime = {
    type: 'line',
    data: {
        labels: [],
        datasets: [{
            label: 'ResponseTime',
            backgroundColor: chartColors.green,
            borderColor: chartColors.green,
            data: [],
            fill: false,
        },{
            label: 'ResponseTime90',
            backgroundColor: chartColors.blue,
            borderColor: chartColors.blue,
            data: [],
            fill: false,
        },{
            label: 'ResponseTime95',
            backgroundColor: chartColors.yellow,
            borderColor: chartColors.yellow,
            data: [],
            fill: false,
        },{
            label: 'ResponseTime99',
            backgroundColor: chartColors.red,
            borderColor: chartColors.red,
            data: [],
            fill: false,
        }]
    },
    options: {
        responsive: true,
        title: {
            display: true,
            text: 'ResponseTime'
        },
        tooltips: {
            mode: 'index',
            intersect: false,
        },
        hover: {
            mode: 'nearest',
            intersect: true
        },
        scales: {
            xAxes: [{
                display: true,
                scaleLabel: {
                    display: true,
                    labelString: 'Time'
                }
            }],
            yAxes: [{
                display: true,
                scaleLabel: {
                    display: true,
                    labelString: 'ResponseTime'
                }
            }]
        }
    }
};

var configErrorTPS = {
    type: 'line',
    data: {
        labels: [],
        datasets: [{
            label: 'TPS',
            backgroundColor: chartColors.red,
            borderColor: chartColors.red,
            data: [],
            fill: false,
        }]
    },
    options: {
        responsive: true,
        title: {
            display: true,
            text: 'error-TPS'
        },
        tooltips: {
            mode: 'index',
            intersect: false,
        },
        hover: {
            mode: 'nearest',
            intersect: true
        },
        scales: {
            xAxes: [{
                display: true,
                scaleLabel: {
                    display: true,
                    labelString: 'Time'
                }
            }],
            yAxes: [{
                display: true,
                scaleLabel: {
                    display: true,
                    labelString: 'TPS'
                }
            }]
        }
    }
};


var configErrorResponseTime = {
    type: 'line',
    data: {
        labels: [],
        datasets: [{
            label: 'ResponseTime',
            backgroundColor: chartColors.green,
            borderColor: chartColors.green,
            data: [],
            fill: false,
        },{
            label: 'ResponseTime90',
            backgroundColor: chartColors.blue,
            borderColor: chartColors.blue,
            data: [],
            fill: false,
        },{
            label: 'ResponseTime95',
            backgroundColor: chartColors.yellow,
            borderColor: chartColors.yellow,
            data: [],
            fill: false,
        },{
            label: 'ResponseTime99',
            backgroundColor: chartColors.red,
            borderColor: chartColors.red,
            data: [],
            fill: false,
        }]
    },
    options: {
        responsive: true,
        title: {
            display: true,
            text: 'Error-ResponseTime'
        },
        tooltips: {
            mode: 'index',
            intersect: false,
        },
        hover: {
            mode: 'nearest',
            intersect: true
        },
        scales: {
            xAxes: [{
                display: true,
                scaleLabel: {
                    display: true,
                    labelString: 'Time'
                }
            }],
            yAxes: [{
                display: true,
                scaleLabel: {
                    display: true,
                    labelString: 'ResponseTime'
                }
            }]
        }
    }
};


var configErrorRate = {
    type: 'line',
    data: {
        labels: [],
        datasets: [{
            label: 'errorRate',
            backgroundColor: chartColors.red,
            borderColor: chartColors.red,
            data: [],
            fill: false,
        }]
    },
    options: {
        responsive: true,
        title: {
            display: true,
            text: 'error-rate'
        },
        tooltips: {
            mode: 'index',
            intersect: false,
        },
        hover: {
            mode: 'nearest',
            intersect: true
        },
        scales: {
            xAxes: [{
                display: true,
                scaleLabel: {
                    display: true,
                    labelString: 'Time'
                }
            }],
            yAxes: [{
                display: true,
                scaleLabel: {
                    display: true,
                    labelString: 'errorRate'
                }
            }]
        }
    }
};


var startTime = 0;

function feed(data) {
    var time = new Date(data.Time*1000);

    if(startTime == 0) {
        startTime = data.Time;
    }
    offset = data.Time - startTime;
    
    if(startTime > data.Time) {
        appendLog("startTime > data.Time," + startTime + "," + data.Time)
        return;
    }

    while (configTPS.data.labels.length < offset) {
        var l = configTPS.data.labels.length
        configTPS.data.labels[l] = l
        configTPS.data.datasets[0].data[l] = 0;
        
        configResponseTime.data.labels[l] = l
        configResponseTime.data.datasets[0].data[l] = 0;
        configResponseTime.data.datasets[1].data[l] = 0;
        configResponseTime.data.datasets[2].data[l] = 0;
        configResponseTime.data.datasets[3].data[l] = 0;
        
        configErrorTPS.data.labels[l] = l
        configErrorTPS.data.datasets[0].data[l] = 0;
        
        configErrorResponseTime.data.labels[l] = l
        configErrorResponseTime.data.datasets[0].data[l] = 0;
        configErrorResponseTime.data.datasets[1].data[l] = 0;
        configErrorResponseTime.data.datasets[2].data[l] = 0;
        configErrorResponseTime.data.datasets[3].data[l] = 0;
    }

    configTPS.data.labels[offset] = offset;
    configTPS.data.datasets[0].data[offset] = data.TpsSuccess;

    configResponseTime.data.labels[offset] = offset;
    configResponseTime.data.datasets[0].data[offset] = data.ResponseTime;
    configResponseTime.data.datasets[1].data[offset] = data.ResponseTime90;
    configResponseTime.data.datasets[2].data[offset] = data.ResponseTime95;
    configResponseTime.data.datasets[3].data[offset] = data.ResponseTime99;

    configErrorTPS.data.labels[offset] = offset;
    configErrorTPS.data.datasets[0].data[offset] = data.TpsFailure;

    configErrorResponseTime.data.labels[offset] = offset;
    configErrorResponseTime.data.datasets[0].data[offset] = data.FailureResponseTime;
    configErrorResponseTime.data.datasets[1].data[offset] = data.FailureResponseTime90;
    configErrorResponseTime.data.datasets[2].data[offset] = data.FailureResponseTime95;
    configErrorResponseTime.data.datasets[3].data[offset] = data.FailureResponseTime99;

    configErrorRate.data.labels[offset] = offset;
    configErrorRate.data.datasets[0].data[offset] = data.ErrorRate;

    window.chartTPS.update();
    window.chartResponseTime.update();
    window.chartErrorTPS.update();
    window.chartErrorResponseTime.update();
    window.chartErrorRate.update();
}

window.onload = function() {
    var ctxTPS = document.getElementById('TPS').getContext('2d');
    window.chartTPS = new Chart(ctxTPS, configTPS);

    var ctxResponseTime = document.getElementById('responseTime').getContext('2d');
    window.chartResponseTime = new Chart(ctxResponseTime, configResponseTime);

    var ctxErrorTPS = document.getElementById('errorTPS').getContext('2d');
    window.chartErrorTPS = new Chart(ctxErrorTPS, configErrorTPS);

    var ctxErrorResponseTime = document.getElementById('errorResponseTime').getContext('2d');
    window.chartErrorResponseTime = new Chart(ctxErrorResponseTime, configErrorResponseTime);

    var ctxErrorRate = document.getElementById('errorRate').getContext('2d');
    window.chartErrorRate = new Chart(ctxErrorRate, configErrorRate);
    
    if (window["WebSocket"]) {
        conn = new WebSocket("ws://localhost:8080/ws");
        conn.onclose = function (evt) {
            appendLog("Connection closed");
        };
        conn.onmessage = function (evt) {
            data = JSON.parse(evt.data);
            feed(data);
        };
    } else {
        var item = document.createElement("div");
        item.innerHTML = "<b>Your browser does not support WebSockets.</b>";
        appendLog(item);
    }
};
</script>
</body>
</html>
`
