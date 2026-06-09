package mapreduce

import (
	"fmt"
	"testing"
)

func TestTmp(t *testing.T) {
	for i := range 10 {
		fmt.Println(i)
	}
}
