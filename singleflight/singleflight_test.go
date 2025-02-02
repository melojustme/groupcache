/*
Copyright 2012 Google Inc.

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

package singleflight

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDo(t *testing.T) {
	var g Group
	v, err := g.Do("key", func() (interface{}, error) {
		return "bar", nil
	})
	if got, want := fmt.Sprintf("%v (%T)", v, v), "bar (string)"; got != want {
		t.Errorf("Do = %v; want %v", got, want)
	}
	if err != nil {
		t.Errorf("Do error = %v", err)
	}
}

func TestDoErr(t *testing.T) {
	var g Group
	someErr := errors.New("some error")
	v, err := g.Do("key", func() (interface{}, error) {
		return nil, someErr
	})
	if err != someErr {
		t.Errorf("Do error = %v; want someErr", err)
	}
	if v != nil {
		t.Errorf("unexpected non-nil value %#v", v)
	}
}

func TestDoDupSuppress(t *testing.T) {
	var g Group
	c := make(chan string)
	var calls int32
	fn := func() (interface{}, error) {
		atomic.AddInt32(&calls, 1)
		return <-c, nil
	}

	const n = 10
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			v, err := g.Do("key", fn)
			if err != nil {
				t.Errorf("Do error: %v", err)
			}
			if v.(string) != "bar" {
				t.Errorf("got %q; want %q", v, "bar")
			}
			wg.Done()
		}()
	}
	time.Sleep(100 * time.Millisecond) // let goroutines above block
	c <- "bar"
	wg.Wait()
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("number of calls = %d; want 1", got)
	}
}

func TestDoPanic(t *testing.T) {
	var g Group
	var err error
	func() {
		defer func() {
			// do not let the panic below leak to the test
			_ = recover()
		}()
		_, err = g.Do("key", func() (interface{}, error) {
			panic("something went horribly wrong")
		})
	}()
	if err != nil {
		t.Errorf("Do error = %v; want someErr", err)
	}
	// ensure subsequent calls to same key still work
	v, err := g.Do("key", func() (interface{}, error) {
		return "foo", nil
	})
	if err != nil {
		t.Errorf("Do error = %v; want no error", err)
	}
	if v.(string) != "foo" {
		t.Errorf("got %q; want %q", v, "foo")
	}
}

func TestDoConcurrentPanic(t *testing.T) {
	var g Group
	c := make(chan struct{})
	var calls int32
	fn := func() (interface{}, error) {
		atomic.AddInt32(&calls, 1)
		<-c
		panic("something went horribly wrong")
	}

	const n = 10
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer func() {
				// do not let the panic leak to the test
				_ = recover()
				wg.Done()
			}()

			v, err := g.Do("key", fn)
			if err == nil || !strings.Contains(err.Error(), "singleflight leader panicked") {
				t.Errorf("Do error: %v; wanted 'singleflight panicked'", err)
			}
			if v != nil {
				t.Errorf("got %q; want nil", v)
			}
		}()
	}
	time.Sleep(100 * time.Millisecond) // let goroutines above block
	c <- struct{}{}
	wg.Wait()
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("number of calls = %d; want 1", got)
	}
}
