/*
Copyright 2018 Craig Johnston <cjimti@gmail.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package utils

import (
	"sync"

	"github.com/txn2/kubefwd/pkg/fwdport"
)

var lock sync.Mutex

func ThreadSafeAppend(a []*fwdport.PortForwardOpts, b ...*fwdport.PortForwardOpts) []*fwdport.PortForwardOpts {
	lock.Lock()
	c := append(a, b...)
	lock.Unlock()
	return c
}
