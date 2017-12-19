// Copyright 2017 New Vector Ltd
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package common

import (
	"sync"
	"testing"
)

func TestSimpleLinearizer(t *testing.T) {
	l := NewLinearizer()

	numCalls := 0
	l.Await("foo", func() {
		numCalls++
	})

	if numCalls != 1 {
		t.Fatalf("Expected function to be called once, called %d times", numCalls)
	}

	activeKeys := l.NumberOfActiveKeys()
	if activeKeys != 0 {
		t.Fatalf("Expected no active keys, got %d active keys", activeKeys)
	}
}

func TestMultipleAfterLinearizer(t *testing.T) {
	l := NewLinearizer()

	numFirstCalls := 0
	l.Await("foo", func() {
		numFirstCalls++
	})

	numSecondCalls := 0
	l.Await("foo", func() {
		numSecondCalls++
	})

	if numFirstCalls != 1 {
		t.Fatalf("Expected first function to be called once, called %d times", numFirstCalls)
	}

	if numSecondCalls != 1 {
		t.Fatalf("Expected second function to be called once, called %d times", numSecondCalls)
	}

	activeKeys := l.NumberOfActiveKeys()
	if activeKeys != 0 {
		t.Fatalf("Expected no active keys, got %d active keys", activeKeys)
	}
}

func TestMultipleConcurrentLinearizer(t *testing.T) { // nolint: gocyclo
	l := NewLinearizer()

	waitGroup := sync.WaitGroup{}
	waitGroup.Add(3)

	numFirstCalls := 0
	numSecondCalls := 0
	numThirdCalls := 0

	startSignal := make(chan struct{})

	setupAwait2 := make(chan struct{})
	setupAwait3 := make(chan struct{})

	go func() {
		l.AwaitWithHook("foo", func() {
			<-startSignal

			numFirstCalls++

			if numFirstCalls != 1 {
				t.Fatalf("Expected first function to be called once, called %d times", numFirstCalls)
			}

			if numSecondCalls != 0 {
				t.Fatalf("Expected second function to not be called, called %d times", numSecondCalls)
			}

			if numThirdCalls != 0 {
				t.Fatalf("Expected third function to not be called, called %d times", numThirdCalls)
			}
		}, setupAwait2)

		t.Log("Finished waiting on w1")
		waitGroup.Done()
	}()

	go func() {
		<-setupAwait2
		l.AwaitWithHook("foo", func() {
			numSecondCalls++

			if numFirstCalls != 1 {
				t.Fatalf("Expected first function to be called once, called %d times", numFirstCalls)
			}

			if numSecondCalls != 1 {
				t.Fatalf("Expected second function to be called once, called %d times", numSecondCalls)
			}

			if numThirdCalls != 0 {
				t.Fatalf("Expected third function to not be called, called %d times", numThirdCalls)
			}
		}, setupAwait3)

		t.Log("Finished waiting on w2")

		waitGroup.Done()
	}()

	go func() {
		<-setupAwait3
		l.AwaitWithHook("foo", func() {
			numThirdCalls++

			if numFirstCalls != 1 {
				t.Fatalf("Expected first function to be called once, called %d times", numFirstCalls)
			}

			if numSecondCalls != 1 {
				t.Fatalf("Expected second function to be called once, called %d times", numSecondCalls)
			}

			if numThirdCalls != 1 {
				t.Fatalf("Expected third function to be called once, called %d times", numThirdCalls)
			}
		}, startSignal)

		t.Log("Finished waiting on w3")
		waitGroup.Done()
	}()

	waitGroup.Wait()

	activeKeys := l.NumberOfActiveKeys()
	if activeKeys != 0 {
		t.Fatalf("Expected no active keys, got %d active keys", activeKeys)
	}
}

func TestDifferentKeysLiniearizer(t *testing.T) {
	l := NewLinearizer()

	waitChan := make(chan struct{})

	numFirstCalls := 0
	go l.Await("foo", func() {
		<-waitChan
		numFirstCalls++
	})

	numSecondCalls := 0
	l.Await("bar", func() {
		numSecondCalls++
	})

	if numFirstCalls != 0 {
		t.Fatalf("Expected first function to not be called, called %d times", numFirstCalls)
	}

	if numSecondCalls != 1 {
		t.Fatalf("Expected second function to be called once, called %d times", numSecondCalls)
	}

	waitChan <- struct{}{}

	if numFirstCalls != 1 {
		t.Fatalf("Expected first function to be called once, called %d times", numFirstCalls)
	}

	if numSecondCalls != 1 {
		t.Fatalf("Expected second function to be called once, called %d times", numSecondCalls)
	}

	activeKeys := l.NumberOfActiveKeys()
	if activeKeys != 0 {
		t.Fatalf("Expected no active keys, got %d active keys", activeKeys)
	}
}
