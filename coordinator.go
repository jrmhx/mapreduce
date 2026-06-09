package mapreduce

import (
	"errors"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"sync"
	"time"
)

type TaskStatus int
type Phase int

const (
	MapPhase Phase = iota
	ReducePhase
	DonePhase
)

const (
	Idle TaskStatus = iota
	Processing
	Done
)

type TaskType int

const (
	MapTask TaskType = iota
	ReduceTask
	DoneTask
	IdleTask
)

type Task struct {
	id       int
	filename string
	start    time.Time
	status   TaskStatus
}

type DoneEvent struct {
	TaskId int
	Type   TaskType
}

type Coordinator struct {
	mu sync.Mutex // guard mptsks and rdtsks
	// Your definitions here.
	mptsks map[int]*Task
	rdtsks map[int]*Task
	phase  Phase
	doneCh chan DoneEvent
}

// Your code here -- RPC handlers for the worker to call.

// the RPC argument and reply types are defined in rpc.go.
func (c *Coordinator) FetchTask(_ *struct{}, reply *FetchTaskReply) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch c.phase {
	case MapPhase:
		reply.Type = MapTask
		reply.NReduce = len(c.rdtsks)
		reply.NMap = len(c.mptsks)
		for tid, t := range c.mptsks {
			if t.status == Idle {
				reply.Id = tid
				reply.Filename = t.filename
				t.start = time.Now()
				t.status = Processing
				return nil
			}
		}
		reply.Type = IdleTask
		return nil
	case ReducePhase:
		reply.Type = ReduceTask
		reply.NReduce = len(c.rdtsks)
		reply.NMap = len(c.mptsks)
		for tid, t := range c.rdtsks {
			if t.status == Idle {
				reply.Id = tid
				t.start = time.Now()
				t.status = Processing
				return nil
			}
		}
		reply.Type = IdleTask
		return nil
	case DonePhase:
		reply.Type = DoneTask
	default:
		reply.Type = IdleTask
		return errors.New("500 coordinator error")
	}

	return nil
}

func (c *Coordinator) ReportDone(args *ReportDoneArgs, _ *ReportDoneReply) error {
	shouldSend := false
	c.mu.Lock()
	switch args.Type {
	case MapTask:
		if c.mptsks[args.TaskId].status != Done {
			shouldSend = true
			c.mptsks[args.TaskId].status = Done
		}
	case ReduceTask:
		if c.rdtsks[args.TaskId].status != Done {
			shouldSend = true
			c.rdtsks[args.TaskId].status = Done
		}
	}
	c.mu.Unlock()

	if shouldSend {
		c.doneCh <- DoneEvent{
			TaskId: args.TaskId,
			Type:   args.Type,
		}
	}
	return nil
}

// start a thread that listens for RPCs from worker.go
func (c *Coordinator) server(sockname string) {
	rpc.Register(c)
	rpc.HandleHTTP()
	os.Remove(sockname)
	l, e := net.Listen("unix", sockname)
	if e != nil {
		log.Fatalf("listen error %s: %v", sockname, e)
	}
	go http.Serve(l, nil)
}

// main/mrcoordinator.go calls Done() periodically to find out
// if the entire job has finished.
func (c *Coordinator) Done() bool {
	ret := false

	// Your code here.
	defer c.mu.Unlock()
	c.mu.Lock()
	if c.phase == DonePhase {
		ret = true
	}

	return ret
}

func (c *Coordinator) PhaseUpdate() {
	mapDone, reduceDone := 0, 0
	// TODO: bug, coordinator blocked when worker crash
	// Need retry
	for ev := range c.doneCh {
		switch ev.Type {
		case MapTask:
			mapDone++
			if mapDone == len(c.mptsks) {
				c.mu.Lock()
				c.phase = ReducePhase
				c.mu.Unlock()
			}
		case ReduceTask:
			reduceDone++
			if reduceDone == len(c.rdtsks) {
				c.mu.Lock()
				c.phase = DonePhase
				c.mu.Unlock()
				return
			}
		}
	}
}

// create a Coordinator.
// main/mrcoordinator.go calls this function.
// nReduce is the number of reduce tasks to use.
func MakeCoordinator(sockname string, files []string, nReduce int) *Coordinator {
	// Your code here.
	nMap := len(files)
	c := Coordinator{}
	c.mptsks = make(map[int]*Task, nMap)
	for id := 0; id < nMap; id++ {
		c.mptsks[id] = &Task{
			id:       id,
			filename: files[id],
			start:    time.Time{},
			status:   Idle,
		}
	}
	c.rdtsks = make(map[int]*Task, nReduce)
	for id := 0; id < nReduce; id++ {
		c.rdtsks[id] = &Task{
			id:       id,
			filename: "",
			start:    time.Time{},
			status:   Idle,
		}
	}
	c.phase = MapPhase
	c.doneCh = make(chan DoneEvent)
	go c.PhaseUpdate()

	c.server(sockname)
	return &c
}
