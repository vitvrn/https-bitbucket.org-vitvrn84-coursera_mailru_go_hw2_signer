package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

//TODO ИСПОЛЬЗОВАТЬ MaxInputDataLen !!!

func myDataSignerMd5(mu *sync.Mutex, data string) (result string) { //??? кто хозяин мьютекса
	mu.Lock()
	defer mu.Unlock()
	result = DataSignerMd5(data)
	return result
}

const (
	timeout1 = time.Millisecond //???
	timeout2 = 3 * time.Second
	chCap    = 8 //чтобы ускорить процесс //???
)

type mhResult struct { //для сбора данных от параллельно работающих горутин
	data  []string
	mutex *sync.Mutex
}

func (mhr *mhResult) setAtomic(i int, v string) {
	if i >= len(mhr.data) {
		fmt.Println("no room for the value to put", i, v)
		return
	}
	mhr.mutex.Lock()
	defer mhr.mutex.Unlock()
	mhr.data[i] = v
}

var (
	start  time.Time     //debug
	finish time.Duration //debug

	SingleHash = func(in, out chan interface{}) {
		md5mutex := &sync.Mutex{}
		wg := &sync.WaitGroup{}
		defer func() {
			wg.Wait() //ждём пока все результаты будут записаны в out
			fmt.Println("### FINISH SingleHash ###")
			close(out)
		}()
		//for1:
		for {
			select {
			case i := <-in: // *-----------------------------------------------
				ii, ok := i.(int)
				if !ok {
					return
				}
				fmt.Println(i, "SingleHash data", i)
				s := strconv.Itoa(ii)

				//c1 := DataSignerCrc32(s)
				ch1 := make(chan string, 1)     //буфер ннада?
				go func(result chan<- string) { //crc32 #1//
					defer close(result)
					result <- DataSignerCrc32(s)
				}(ch1)

				//m1 := DataSignerMd5(s)
				//c2 := DataSignerCrc32(m1)
				ch2 := make(chan string, 1)
				go func(result chan<- string) {
					defer close(result)
					//m1 := DataSignerMd5(s)
					m1 := myDataSignerMd5(md5mutex, s)
					fmt.Println(i, "SingleHash md5(data)", m1)
					result <- DataSignerCrc32(m1)
				}(ch2)

				wg.Add(1)
				go func(wg *sync.WaitGroup) { //(md5 -> crc32 #2) //
					defer wg.Done()
					c1, c2 := <-ch1, <-ch2
					res := c1 + "~" + c2
					fmt.Println(i, "SingleHash crc32(md5(data))", c2)
					fmt.Println(i, "SingleHash crc32(data)", c1)
					fmt.Println(i, "SingleHash result", res)
					out <- res
				}(wg)
			//case out <- res //??? чтобы не было задержки при записи в out
			case <-time.After(timeout1):
				return //break for1
			}
		}
	}

	//TODO если распараллелим  расчет crc32(th+data), то нужно потом отсортировать перед окончательной конкатенацией по какому-либо признаку
	MultiHash = func(in, out chan interface{}) {
		wg := &sync.WaitGroup{}
		defer func(wg *sync.WaitGroup) {
			wg.Wait() //ждём пока все результаты будут записаны в out
			fmt.Println("### FINISH MultiHash ###")
			close(out)
		}(wg)
		//mhfor:
		for val := range in {
			s, ok := val.(string)
			if !ok {
				return //break mhfor ???
			}
			result := mhResult{
				data:  make([]string, 6),
				mutex: &sync.Mutex{},
			}
			forWG := &sync.WaitGroup{}

			for i := 0; i <= 5; i++ {
				ss := strconv.Itoa(i) + s

				forWG.Add(1)
				go func(forWG *sync.WaitGroup, i int, s, ss string) { //crc32 #3//
					defer forWG.Done()
					sss := DataSignerCrc32(ss)
					result.setAtomic(i, sss)
					fmt.Println(s, "MultiHash: crc32(th+step1))", i, sss)
				}(forWG, i, s, ss)
			}
			wg.Add(1)
			go func(wg, forWG *sync.WaitGroup, s string) { //assemble crc32_result{0..5} //
				defer wg.Done()
				forWG.Wait() //ждём 6 параллельных расчетов
				resultstr := strings.Join(result.data, "")
				fmt.Println(s, "MultiHash result:\n", resultstr)
				out <- resultstr
			}(wg, forWG, s)
		}
	}

	CombineResults = func(in, out chan interface{}) {
		defer func() {
			finish = time.Since(start)
			fmt.Println("### FINISH CombineResults ###", finish)
			close(out) //???
		}()
		ins := make([]string, 0, 2) //cap=100 ???
	for1:
		for s := range in {
			ss, ok := s.(string)
			if !ok {
				break for1
			}
			ins = append(ins, ss)
		}
		sort.StringSlice(ins).Sort()
		res := strings.Join(ins, "_")
		fmt.Println("CombineResults", res)
		out <- res
	}
)

func ExecutePipeline(jobs ...job) {
	var in, out chan interface{}
	out = make(chan interface{}, chCap)
	start = time.Now()
	mainwg := &sync.WaitGroup{}
	for i, _ := range jobs {
		in, out = out, make(chan interface{}, chCap)
		mainwg.Add(1)
		go func(i int, wg *sync.WaitGroup, in, out chan interface{}) {
			defer wg.Done()
			//fmt.Printf("\n++ JOB #%v: %v, in=%v, out=%v ++\n", i, jobs[i], in, out)
			jobs[i](in, out)
			//fmt.Printf("\n-- JOB #%v: %v, in=%v, out=%v --\n", i, jobs[i], in, out)
		}(i, mainwg, in, out)
	}

	ready := make(chan struct{})
	go func(wg *sync.WaitGroup, ch chan struct{}) { //ждём вне основного потока
		wg.Wait()
		ready <- struct{}{}
	}(mainwg, ready)

	select {
	case <-ready:
		fmt.Println("### ExecutePipeline ### All done ###")
	case <-time.After(timeout2):
		fmt.Println("### ExecutePipeline ### Cancel execution ###")
	}
}
