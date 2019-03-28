package controller

import (
	"github.com/AsherShoshan/cnv-svc4vmi-operator/pkg/controller/ctl"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, ctl.Add)
}
