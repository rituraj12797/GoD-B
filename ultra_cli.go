package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// TEMPORARY: comment out main for benchmark run
func main() {
	const dbFile = "ultra_interactive.db.btree"
	const walFile = "ultra_interactive.db.wal"
	kv, err := NewUltraKV(dbFile, walFile)
	if err != nil {
		fmt.Printf("Failed to open UltraKV: %v\n", err) // [DEBUG]
		return
	}
	defer kv.Close()

	fmt.Println(`
 $$$$$$\   $$$$$$\  $$$$$$$\          $$$$$$$\  
$$  __$$\ $$  __$$\ $$  __$$\         $$  __$$\ 
$$ /  \__|$$ /  $$ |$$ |  $$ |        $$ |  $$ |
$$ |$$$$\ $$ |  $$ |$$ |  $$ |$$$$$$\ $$$$$$$\ |
$$ |\_$$ |$$ |  $$ |$$ |  $$ |\______|$$  __$$\ 
$$ |  $$ |$$ |  $$ |$$ |  $$ |        $$ |  $$ |
\$$$$$$  | $$$$$$  |$$$$$$$  |        $$$$$$$  |
 \______/  \______/ \_______/         \_______/ 
                 
                                                
                                                `)
	fmt.Println("UltraKV CLI. Commands: set <k> <v>, get <k>, del <k>, begin, commit, abort, debug, clear, exit") // [DEBUG]
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("> ") // [DEBUG]
		line, err := reader.ReadString('\n')
		if err != nil {
			// fmt.Println("Error reading input:", err)
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}
		switch parts[0] {
		case "set":
			if len(parts) < 3 {
				// fmt.Println("Usage: set <key> <value>")
				continue
			}
			kv.Set(parts[1], strings.Join(parts[2:], " "))
			// Force immediate flush for CLI operations
			kv.flushCh <- struct{}{}
		case "get":
			if len(parts) != 2 {
				// fmt.Println("Usage: get <key>")
				continue
			}
			v, ok := kv.Get(parts[1])
			if ok {
				fmt.Printf("%q\n", v)
			} else {
				fmt.Println("(nil)")
			}
		case "del":
			if len(parts) != 2 {
				// fmt.Println("Usage: del <key>")
				continue
			}
			kv.Del(parts[1])
			// Force immediate flush for CLI operations
			kv.flushCh <- struct{}{}
		case "begin":
			kv.Begin()
		case "commit":
			kv.Commit()
		case "abort":
			kv.Abort()
		case "debug":
			kv.DebugPrint()
		case "exit", "quit":
			// fmt.Println("Exiting.") // [DEBUG]
			return
		case "clear":
			fmt.Print("Are you sure you want to clear the database? (Y/N): ")
			confirm, _ := reader.ReadString('\n')
			confirm = strings.TrimSpace(confirm)
			if confirm == "Y" || confirm == "y" {
				err := kv.Clear()
				if err != nil {
					fmt.Println("Error clearing database:", err)
				} else {
					fmt.Println("Database cleared.")
				}
			} else {
				fmt.Println("Clear cancelled.")
			}
			continue
		default:
			fmt.Println("Unknown command.")
		}
	}
}
