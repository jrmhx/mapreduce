package mapreduce

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"net/rpc"
	"os"
	"sort"
	"time"
)

// Map functions return a slice of KeyValue.
type KeyValue struct {
	Key   string
	Value string
}

// use ihash(key) % NReduce to choose the reduce
// task number for each KeyValue emitted by Map.
func ihash(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() & 0x7fffffff)
}

var coordSockName string // socket for coordinator
var currentMapf func(string, string) []KeyValue
var currentReducef func(string, []string) string

type ByKey []KeyValue

func (a ByKey) Len() int           { return len(a) }
func (a ByKey) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByKey) Less(i, j int) bool { return a[i].Key < a[j].Key }

// main/mrworker.go calls this function.
func Worker(sockname string, mapf func(string, string) []KeyValue,
	reducef func(string, []string) string) {

	coordSockName = sockname
	currentMapf = mapf
	currentReducef = reducef

	for {
		task, err := CallFetchTask()
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		switch task.Type {
		case MapTask:
			if err := Map(task); err != nil {
				log.Printf("map task %d failed: %v", task.Id, err)
				time.Sleep(100 * time.Millisecond)
				continue
			}
			for {
				if err := CallReportDone(task.Id, task.Type); err == nil {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
		case ReduceTask:
			if err := Reduce(task); err != nil {
				log.Printf("reduce task %d failed: %v", task.Id, err)
				time.Sleep(100 * time.Millisecond)
				continue
			}
			for {
				if err := CallReportDone(task.Id, task.Type); err == nil {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
		case DoneTask:
			return
		case IdleTask:
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// example function to show how to make an RPC call to the coordinator.
//
// the RPC argument and reply types are defined in rpc.go.
func CallExample() {

	// declare an argument structure.
	args := ExampleArgs{}

	// fill in the argument(s).
	args.X = 99

	// declare a reply structure.
	reply := ExampleReply{}

	// send the RPC request, wait for the reply.
	// the "Coordinator.Example" tells the
	// receiving server that we'd like to call
	// the Example() method of struct Coordinator.
	ok := call("Coordinator.Example", &args, &reply)
	if ok {
		// reply.Y should be 100.
		fmt.Printf("reply.Y %v\n", reply.Y)
	} else {
		fmt.Printf("call failed!\n")
	}
}

func CallFetchTask() (FetchTaskReply, error) {
	reply := FetchTaskReply{}
	args := struct{}{}

	ok := call("Coordinator.FetchTask", &args, &reply)

	if ok {
		return reply, nil
	} else {
		return reply, errors.New("fectch task failed")
	}
}

func CallReportDone(tid int, ttype TaskType) error {
	args := ReportDoneArgs{
		tid,
		ttype,
	}
	reply := ReportDoneReply{}

	ok := call("Coordinator.ReportDone", &args, &reply)
	if ok {
		return nil
	} else {
		return errors.New("report done failed")
	}
}

func Map(task FetchTaskReply) error {
	if task.NReduce <= 0 {
		return errors.New("missing reduce count")
	}

	file, err := os.Open(task.Filename)
	if err != nil {
		return err
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	kva := currentMapf(task.Filename, string(content))
	buckets := make([][]KeyValue, task.NReduce)
	for _, kv := range kva {
		reduceID := ihash(kv.Key) % task.NReduce
		buckets[reduceID] = append(buckets[reduceID], kv)
	}

	for reduceID, bucket := range buckets {
		if err := writeIntermediateFile(task.Id, reduceID, bucket); err != nil {
			return err
		}
	}

	return nil
}

func Reduce(task FetchTaskReply) error {
	kva := make([]KeyValue, 0)

	for mapID := 0; mapID < task.NMap; mapID++ {
		filename := fmt.Sprintf("mr-%d-%d", mapID, task.Id)
		file, err := os.Open(filename)
		if err != nil {
			return err
		}

		dec := json.NewDecoder(file)
		for {
			var kv KeyValue
			if err := dec.Decode(&kv); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				file.Close()
				return err
			}
			kva = append(kva, kv)
		}
		file.Close()
	}

	sort.Sort(ByKey(kva))

	oname := fmt.Sprintf("mr-out-%d", task.Id)
	tmp, err := os.CreateTemp(".", fmt.Sprintf("mr-out-%d-*", task.Id))
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	i := 0
	for i < len(kva) {
		j := i + 1
		for j < len(kva) && kva[j].Key == kva[i].Key {
			j++
		}
		values := make([]string, 0, j-i)
		for k := i; k < j; k++ {
			values = append(values, kva[k].Value)
		}
		output := currentReducef(kva[i].Key, values)
		if _, err := fmt.Fprintf(tmp, "%v %v\n", kva[i].Key, output); err != nil {
			tmp.Close()
			return err
		}
		i = j
	}

	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Remove(oname); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(tmp.Name(), oname)
}

func writeIntermediateFile(mapID, reduceID int, kvs []KeyValue) error {
	oname := fmt.Sprintf("mr-%d-%d", mapID, reduceID)
	tmp, err := os.CreateTemp(".", fmt.Sprintf("mr-%d-%d-*", mapID, reduceID))
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	enc := json.NewEncoder(tmp)
	for _, kv := range kvs {
		if err := enc.Encode(&kv); err != nil {
			tmp.Close()
			return err
		}
	}

	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Remove(oname); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(tmp.Name(), oname)
}

// send an RPC request to the coordinator, wait for the response.
// usually returns true.
// returns false if something goes wrong.
func call(rpcname string, args interface{}, reply interface{}) bool {
	// c, err := rpc.DialHTTP("tcp", "127.0.0.1"+":1234")
	c, err := rpc.DialHTTP("unix", coordSockName)
	if err != nil {
		log.Fatal("dialing:", err)
	}
	defer c.Close()

	if err := c.Call(rpcname, args, reply); err == nil {
		return true
	}
	log.Printf("%d: call failed err %v", os.Getpid(), err)
	return false
}
