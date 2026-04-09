package relay

import (
    "log"
    "os"
)

func relayEvalEnabled() bool {
    return os.Getenv("TDID_EVAL_METRICS") == "1"
}

func relayEvalLog(format string, args ...any) {
    if !relayEvalEnabled() {
        return
    }
    log.Printf("[tdid-eval][relay] "+format, args...)
}
