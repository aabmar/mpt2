// ble-search: scan for BLE devices and print MAC and name
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"tinygo.org/x/bluetooth"
)

func main() {
	timeout := flag.Duration("timeout", 8*time.Second, "scan duration, e.g. 5s, 10s")
	duplicates := flag.Bool("duplicates", false, "print duplicates as they are discovered")
	showRSSI := flag.Bool("rssi", true, "show RSSI in output")
	progress := flag.Bool("progress", true, "show scan progress on stderr")
	flag.Parse()

	adapter := bluetooth.DefaultAdapter
	if err := adapter.Enable(); err != nil {
		fmt.Fprintf(os.Stderr, "Error enabling BLE adapter: %v\n", err)
		os.Exit(1)
	}

	type info struct {
		addr string
		name string
		rssi int16
	}
	seen := make(map[string]info)
	var mu sync.RWMutex

	if *progress {
		fmt.Fprintf(os.Stderr, "Starting BLE scan for %s...\n", timeout.String())
	}

	if err := adapter.Scan(func(a *bluetooth.Adapter, result bluetooth.ScanResult) {
		addr := result.Address.String()
		name := result.LocalName()
		if name == "" {
			name = "(no name)"
		}
		if !*duplicates {
			mu.RLock()
			_, ok := seen[addr]
			mu.RUnlock()
			if ok {
				return
			}
		}
		entry := info{addr: addr, name: name, rssi: result.RSSI}

		// if result.HasServiceUUID() {
		// 	// If the device has a service UUID, we can use it to filter or identify devices.
		// 	// For now, we just print it.
		// 	fmt.Printf("Has our service UUID: %s\n", result.HasServiceUUID(XXX));
		// }

		mu.Lock()
		seen[addr] = entry
		mu.Unlock()

		if *showRSSI {
			fmt.Printf("%s\t%s\t(RSSI %d)\n", addr, name, result.RSSI)
		} else {
			fmt.Printf("%s\t%s\n", addr, name)
		}
	}); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting scan: %v\n", err)
		os.Exit(1)
	}

	// Let it scan for the requested duration
	start := time.Now()
	var ticker *time.Ticker
	done := make(chan struct{})
	if *progress {
		ticker = time.NewTicker(1 * time.Second)
		go func() {
			for {
				select {
				case <-ticker.C:
					mu.RLock()
					count := len(seen)
					mu.RUnlock()
					elapsed := time.Since(start).Truncate(time.Second)
					// carriage return to update same line; pad spaces to clear
					fmt.Fprintf(os.Stderr, "\rScanning... %s elapsed, %d device(s) found        ", elapsed, count)
				case <-done:
					return
				}
			}
		}()
	}
	time.Sleep(*timeout)
	_ = adapter.StopScan()
	if *progress {
		close(done)
		ticker.Stop()
		mu.RLock()
		total := len(seen)
		mu.RUnlock()
		fmt.Fprintln(os.Stderr) // newline after \r line
		fmt.Fprintf(os.Stderr, "Scan complete: %d device(s) found.\n", total)
	}

	// Print unique results if not already printed as duplicates
	if !*duplicates {
		mu.RLock()
		addrs := make([]string, 0, len(seen))
		for k := range seen {
			addrs = append(addrs, k)
		}
		mu.RUnlock()
		sort.Strings(addrs)
		for _, a := range addrs {
			mu.RLock()
			e := seen[a]
			mu.RUnlock()
			if *showRSSI {
				fmt.Printf("%s\t%s\t(RSSI %d)\n", e.addr, e.name, e.rssi)
			} else {
				fmt.Printf("%s\t%s\n", e.addr, e.name)
			}
		}
	}
}
