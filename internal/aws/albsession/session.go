package albsession

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/ticketmaster/aws-sdk-go-cache/cache"
	"github.com/ticketmaster/aws-sdk-go-cache/timing"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/metric"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
)

// NewSession returns an AWS session based off of the provided AWS config
func NewSession(awsconfig *aws.Config, AWSDebug bool, mc metric.Collector, cc *cache.Config) *session.Session {
	session, err := session.NewSession(awsconfig)
	if err != nil {
		mc.IncAPIErrorCount(prometheus.Labels{"service": "AWS", "request": "NewSession"})
		glog.ErrorDepth(4, fmt.Sprintf("Failed to create AWS session: %s", err.Error()))
		return nil
	}

	// Adds caching to session
	cache.AddCaching(session, cc)
	timing.AddTiming(session)

	session.Handlers.Retry.PushFront(func(r *request.Request) {
		mc.IncAPIRetryCount(prometheus.Labels{"service": r.ClientInfo.ServiceName, "operation": r.Operation.Name})
	})

	session.Handlers.Send.PushFront(func(r *request.Request) {
		mc.IncAPIRequestCount(prometheus.Labels{"service": r.ClientInfo.ServiceName, "operation": r.Operation.Name})
		if AWSDebug {
			glog.InfoDepth(4, fmt.Sprintf("Request: %s/%s, Payload: %s", r.ClientInfo.ServiceName, r.Operation.Name, log.Prettify(r.Params)))
		}
	})

	session.Handlers.Complete.PushFront(func(r *request.Request) {
		ctx := r.HTTPRequest.Context()
		td := timing.GetData(ctx)

		glog.Infof("cached [%v] service [%s.%s] duration [%v]\n",
			cache.IsCacheHit(ctx),
			r.ClientInfo.ServiceName,
			r.Operation.Name,
			td.RequestDuration(),
		)

		if r.Error != nil {
			mc.IncAPIErrorCount(prometheus.Labels{"service": r.ClientInfo.ServiceName, "operation": r.Operation.Name})
			if AWSDebug {
				glog.ErrorDepth(4, fmt.Sprintf("Failed request: %s/%s, Payload: %s, Error: %s", r.ClientInfo.ServiceName, r.Operation.Name, log.Prettify(r.Params), r.Error))
			}
		}
	})
	return session
}
