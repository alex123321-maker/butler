package main
import("encoding/json";"os")
func main(){json.NewEncoder(os.Stdout).Encode(struct{PID int `json:"pid"`; Environment []string `json:"environment"`}{os.Getpid(),os.Environ()});os.Exit(42)}