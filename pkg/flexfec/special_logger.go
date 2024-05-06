package flexfec

import "fmt"

const specialLogEnabled = true

func specialLog(toLog ...interface{}) {
	if specialLogEnabled {
		fmt.Println(toLog...)
	}
}
