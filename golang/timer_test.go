package fastway

import (
	"testing"
	"time"

	"github.com/funny/utest"
)

func Test_TimingWheel(t *testing.T) {
	w := newTimingWheel(100*time.Millisecond, 10)
	defer w.Stop()

	<-w.After(200 * time.Millisecond)

	var err interface{}
	func() {
		defer func() {
			err = recover()
		}()
		w.After(2 * time.Second)
	}()
	utest.NotNilNow(t, err)
}
