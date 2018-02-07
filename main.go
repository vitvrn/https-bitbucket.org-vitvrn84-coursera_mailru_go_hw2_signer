package main

func main() {
	println("run as\n\ngo test -v -race")

	var f job
	f = func(in, out chan interface{}) {
		for i := 0; i <= 1; i++ {
			out <- i
		}
		close(out)
	}
	ExecutePipeline(f, SingleHash, MultiHash, CombineResults)

	//in, out := make(chan interface{}), make(chan interface{})
	//SingleHash(in, out)
	//MultiHash(in, out)
}
