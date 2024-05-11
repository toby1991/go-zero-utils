package nsq

import (
	"context"
	"errors"
	"fmt"
	"github.com/toby1991/go-zero-utils/queue"
	"reflect"
	"testing"
	"time"
)

type DelayGoodsKlineDataFillingJobData struct {
	GoodsId   uint64 `json:"goodsId"`   // 商品id
	SiteId    uint64 `json:"siteId"`    // 站点id
	KlineType string `json:"klineType"` // 5m, 15m, 30m, 1h

	// k线柱子
	KTime uint64 `json:"kTime"` // k线柱子时间
}

func Test_nsqClient_Push(t *testing.T) {
	type fields struct {
		_conf               NsqConf
		senderPool          *ProducerPool
		jobNameProcessorMap map[string]queue.JobProcessor
		ctx                 context.Context
	}
	type args struct {
		job *queue.Job
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "test",
			fields: fields{
				_conf: NsqConf{
					Sender: SenderConf{
						NsqdAddrs: []string{"dev.xxx.com:32682"},
					},
					Worker: WorkerConf{
						NsqLookupdAddrs:            []string{"dev.xxx.com:32006"},
						MaxInFlight:                50,
						PullFromQueuesWithPriority: map[string]int{"delay_ag_goods_kline_data_filling": 1},
					},
				},
			},
			args: args{
				job: func() *queue.Job {
					j := queue.NewJob("delay_ag_goods_kline_data_filling", &DelayGoodsKlineDataFillingJobData{
						GoodsId:   1,
						SiteId:    1,
						KlineType: "5m",
						KTime:     1,
					})
					j.Queue = "delay_ag_goods_kline_data_filling"
					j.At = time.Now().Add(time.Minute * (5 + 1)).Format(time.RFC3339Nano)

					return j
				}(),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewNsq(tt.fields._conf)
			if err := c.Push(tt.args.job); (err != nil) != tt.wantErr {
				t.Errorf("Push() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_nsqClient_Start(t *testing.T) {
	type fields struct {
		_conf                           NsqConf
		jobTopicChannelMapWithProcessor map[Topic]ChannelProcessorMap
	}
	tests := []struct {
		name   string
		fields fields
	}{
		{
			name: "test",
			fields: fields{
				_conf: NsqConf{
					Sender: SenderConf{
						NsqdAddrs: []string{"dev.xxx.com:32682"},
					},
					Worker: WorkerConf{
						NsqLookupdAddrs:            []string{"dev.xxx.com:32006"},
						MaxInFlight:                50,
						PullFromQueuesWithPriority: map[string]int{"delay_ag_goods_kline_data_filling": 1},
					},
				},
				jobTopicChannelMapWithProcessor: map[Topic]ChannelProcessorMap{
					"delay_ag_goods_kline_data_filling": {
						"delay_ag_goods_kline_data_filling": func(helper queue.Helper, args ...interface{}) error {
							fmt.Println("delay_ag_goods_kline_data_filling", args, helper.Jid(), helper.JobType())
							return nil
						},
					},
				},
			},
		},
		{
			name: "test ag_goods_5m",
			fields: fields{
				_conf: NsqConf{
					Sender: SenderConf{
						NsqdAddrs: []string{"dev.xxx.com:32682"},
					},
					Worker: WorkerConf{
						NsqLookupdAddrs:            []string{"dev.xxx.com:32006"},
						MaxInFlight:                50,
						PullFromQueuesWithPriority: map[string]int{"ag_goods_5m": 1},
					},
				},
				jobTopicChannelMapWithProcessor: map[Topic]ChannelProcessorMap{
					"ag_goods_5m": {
						"ag_goods_5m": func(helper queue.Helper, args ...interface{}) error {
							fmt.Println(reflect.TypeOf(args[0]).String())
							fmt.Println(args[0])
							fmt.Println("ag_goods_5m", helper.Jid(), helper.JobType())
							return errors.New("test error")
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewNsq(tt.fields._conf)
			c.SetProcessor(tt.fields.jobTopicChannelMapWithProcessor)
			c.Start()

			time.Sleep(time.Hour * 1)
		})
	}
}
