package chain

import (
    "log"
    "os"
)

func chainEvalEnabled() bool {
    return os.Getenv("TDID_EVAL_METRICS") == "1"
}

func chainEvalLog(format string, args ...any) {
    if !chainEvalEnabled() {
        return
    }
    log.Printf("[tdid-eval][chain] "+format, args...)
}
