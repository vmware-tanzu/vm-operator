/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package record

import (
	"regexp"
	"time"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	controllererror "sigs.k8s.io/cluster-api/pkg/controller/error"
)

const (
	// reason should be Success/Failure+OpName, eg. SuccessfulCreate, FailedCreate, etc.
	Success = "Successful"
	Failure = "Failed"

	RecorderName = "vm-operator"
)

func EmitEvent(object runtime.Object, opName string, err *error, ignoresuccess bool) {
	if *err == nil {
		if !ignoresuccess {
			Event(object, Success+opName, opName+" success")
		}
	} else if _, ok := errors.Cause(*err).(controllererror.HasRequeueAfterError); ok {
		return
	} else {
		Warn(object, Failure+opName, (*err).Error())
	}
}

func ReadEvents(er *record.FakeRecorder) map[string]int {
	reasonMap := make(map[string]int)

	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
loop:
	for {
		select {
		case event := <-er.Events:
			key := getKey(event)
			reasonMap[key] += 1
		case <-timer.C:
			break loop
		}
	}
	return reasonMap
}

func getKey(event string) string {
	r := regexp.MustCompile("(?:" + Success + "|" + Failure + ")[^ ]*")
	key := r.FindString(event)
	return key
}
