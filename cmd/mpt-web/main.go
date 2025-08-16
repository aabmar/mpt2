// mpt-web: Web server for thermal printer with GUI and API endpoints
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/aabmar/mpt2/go/pkg/discovery"
	"github.com/aabmar/mpt2/go/pkg/printer"
	"github.com/aabmar/mpt2/go/pkg/printing"
	"github.com/sirupsen/logrus"
)

// PrintRequest represents the JSON payload for the API endpoint
type PrintRequest struct {
	Text string `json:"text"`
}

// PrintResponse represents the JSON response from the API endpoint
type PrintResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>MPT Thermal Printer</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            max-width: 800px;
            margin: 0 auto;
            padding: 20px;
            background-color: #f5f5f5;
        }
        .container {
            background: white;
            padding: 30px;
            border-radius: 8px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
        }
        h1 {
            color: #333;
            text-align: center;
            margin-bottom: 30px;
        }
        textarea {
            width: 100%;
            height: 200px;
            padding: 15px;
            border: 2px solid #ddd;
            border-radius: 4px;
            font-family: 'Courier New', monospace;
            font-size: 14px;
            resize: vertical;
            box-sizing: border-box;
        }
        textarea:focus {
            outline: none;
            border-color: #4CAF50;
        }
        .button-container {
            text-align: center;
            margin-top: 20px;
        }
        button {
            background-color: #4CAF50;
            color: white;
            padding: 12px 30px;
            border: none;
            border-radius: 4px;
            cursor: pointer;
            font-size: 16px;
            transition: background-color 0.3s;
        }
        button:hover {
            background-color: #45a049;
        }
        button:disabled {
            background-color: #cccccc;
            cursor: not-allowed;
        }
        .message {
            margin-top: 20px;
            padding: 10px;
            border-radius: 4px;
            text-align: center;
        }
        .success {
            background-color: #d4edda;
            color: #155724;
            border: 1px solid #c3e6cb;
        }
        .error {
            background-color: #f8d7da;
            color: #721c24;
            border: 1px solid #f5c6cb;
        }
        .info {
            margin-bottom: 20px;
            padding: 15px;
            background-color: #e7f3ff;
            border-left: 4px solid #2196F3;
            border-radius: 4px;
        }
        .info h3 {
            margin-top: 0;
            color: #1976D2;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>🖨️ MPT Thermal Printer</h1>
        
        <div class="info">
            <h3>How to use:</h3>
            <p>Enter your text below using Markdown formatting. Supported features include:</p>
            <ul>
                <li><strong>Bold text</strong> with **bold** or __bold__</li>
                <li><em>Italic text</em> with *italic* or _italic_</li>
                <li># Headers (1-6 levels)</li>
                <li>- Bullet lists</li>
                <li>1. Numbered lists</li>
                <li>> Blockquotes</li>
                <li>` + "`code`" + ` inline code</li>
                <li>[Links](url) and ![Images](url)</li>
                <li>--- Horizontal rules</li>
            </ul>
        </div>

        <form id="printForm">
            <textarea id="textContent" name="text" placeholder="Enter your text here using Markdown formatting...

Example:
# Hello World!
This is **bold** text and this is *italic*.

## Shopping List
- Milk
- Bread
- Eggs

> This is a quote

` + "`code example`" + `"></textarea>
            
            <div class="button-container">
                <button type="submit" id="printButton">🖨️ Print</button>
            </div>
        </form>

        <div id="message" class="message" style="display: none;"></div>
    </div>

    <script>
        document.getElementById('printForm').addEventListener('submit', async function(e) {
            e.preventDefault();
            
            const textContent = document.getElementById('textContent').value;
            const button = document.getElementById('printButton');
            const messageDiv = document.getElementById('message');
            
            if (!textContent.trim()) {
                showMessage('Please enter some text to print.', 'error');
                return;
            }
            
            button.disabled = true;
            button.textContent = '🖨️ Printing...';
            
            try {
                const response = await fetch('/api/print', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                    },
                    body: JSON.stringify({ text: textContent })
                });
                
                const result = await response.json();
                
                if (response.ok && result.success) {
                    showMessage('✅ Successfully printed!', 'success');
                } else {
                    showMessage('❌ Error: ' + (result.error || 'Unknown error'), 'error');
                }
            } catch (error) {
                showMessage('❌ Network error: ' + error.message, 'error');
            } finally {
                button.disabled = false;
                button.textContent = '🖨️ Print';
            }
        });
        
        function showMessage(text, type) {
            const messageDiv = document.getElementById('message');
            messageDiv.textContent = text;
            messageDiv.className = 'message ' + type;
            messageDiv.style.display = 'block';
            
            setTimeout(() => {
                messageDiv.style.display = 'none';
            }, 5000);
        }
    </script>
</body>
</html>`

var (
	manager        *discovery.ConnectionManager
	thermalPrinter *printer.ThermalPrinter
	printerMutex   sync.Mutex
)

func main() {
	// Command line flags
	port := flag.Int("port", 8080, "Port to run the web server on")
	width := flag.Int("width", 32, "Line width in characters (e.g., 32 for 58mm)")
	feed := flag.Int("feed", 2, "Number of lines to feed after printing")
	cut := flag.Bool("cut", false, "Cut paper after printing")
	codepage := flag.Int("codepage", -1, "ESC/POS code page (0=PC437,2=PC850,5=PC865,16=WPC1252,19=PC858)")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")
	help := flag.Bool("help", false, "Show help")
	flag.Parse()

	if *help {
		usage()
		return
	}

	// Configure logging
	if *verbose {
		logrus.SetLevel(logrus.DebugLevel)
	} else {
		logrus.SetLevel(logrus.InfoLevel)
	}

	// Initialize connection manager
	manager = discovery.NewConnectionManager()

	// Connect to printer once at startup
	logrus.Info("Connecting to printer at startup...")
	var err error
	thermalPrinter, err = manager.ConnectUSB(context.Background(), 0x0483, 0x5840, *codepage)
	if err != nil {
		logrus.Fatalf("Failed to connect to printer at startup: %v", err)
	}
	logrus.Info("Successfully connected to printer!")

	// Create markdown options
	markdownOpts := printing.MarkdownOptions{
		LineWidth: *width,
		FeedLines: *feed,
		Cut:       *cut,
	}

	// Setup HTTP routes
	http.HandleFunc("/", handleHome)
	http.HandleFunc("/api/print", func(w http.ResponseWriter, r *http.Request) {
		handleAPIPrint(w, r, markdownOpts, *codepage)
	})

	// Create server
	server := &http.Server{
		Addr:    ":" + strconv.Itoa(*port),
		Handler: nil,
	}

	// Start server in goroutine
	go func() {
		logrus.Infof("Starting MPT web server on port %d", *port)
		fmt.Printf("🖨️  MPT Thermal Printer Web Server\n")
		fmt.Printf("📡 Server running on: http://localhost:%d\n", *port)
		fmt.Printf("🌐 Click the link above to open in your browser\n")
		fmt.Printf("⏹️  Press Ctrl+C to stop\n\n")

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logrus.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	// Graceful shutdown
	logrus.Info("Shutting down server...")
	fmt.Println("\n⏹️  Shutting down server...")

	// Disconnect printer
	if thermalPrinter != nil {
		logrus.Info("Disconnecting printer...")
		thermalPrinter.Disconnect()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logrus.Errorf("Server shutdown error: %v", err)
	} else {
		fmt.Println("✅ Server stopped gracefully")
	}
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	tmpl, err := template.New("home").Parse(htmlTemplate)
	if err != nil {
		logrus.Errorf("Template parsing error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if err := tmpl.Execute(w, nil); err != nil {
		logrus.Errorf("Template execution error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

func handleAPIPrint(w http.ResponseWriter, r *http.Request, opts printing.MarkdownOptions, codepage int) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	var req PrintRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logrus.Warnf("Invalid JSON in print request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(PrintResponse{
			Success: false,
			Error:   "Invalid JSON format",
		})
		return
	}

	if req.Text == "" {
		logrus.Warn("Empty text in print request")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(PrintResponse{
			Success: false,
			Error:   "Text content is required",
		})
		return
	}

	// Use persistent printer connection with mutex for thread safety
	printerMutex.Lock()
	defer printerMutex.Unlock()

	if thermalPrinter == nil {
		logrus.Error("Printer not connected")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(PrintResponse{
			Success: false,
			Error:   "Printer not connected",
		})
		return
	}

	// Print markdown
	logrus.Infof("Printing text via web API")
	if err := printing.PrintMarkdown(thermalPrinter, req.Text, opts); err != nil {
		logrus.Errorf("Failed to print: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(PrintResponse{
			Success: false,
			Error:   "Failed to print: " + err.Error(),
		})
		return
	}

	logrus.Info("Print completed successfully")
	json.NewEncoder(w).Encode(PrintResponse{
		Success: true,
		Message: "Successfully printed",
	})
}

func usage() {
	fmt.Println("mpt-web - Web server for MPT thermal printer")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Printf("  %s [options]\n", os.Args[0])
	fmt.Println("")
	fmt.Println("Options:")
	fmt.Println("  -port N      Port to run web server on (default: 8080)")
	fmt.Println("  -width N     Line width in characters (default: 32)")
	fmt.Println("  -feed N      Lines to feed after printing (default: 2)")
	fmt.Println("  -cut         Cut paper after printing")
	fmt.Println("  -codepage N  ESC/POS code page (0=PC437,2=PC850,5=PC865,16=WPC1252,19=PC858)")
	fmt.Println("  -verbose     Enable verbose logging")
	fmt.Println("  -help        Show this help")
	fmt.Println("")
	fmt.Println("The server provides:")
	fmt.Println("  /          - Web GUI for printing")
	fmt.Println("  /api/print - JSON API endpoint for printing")
	fmt.Println("")
	fmt.Println("API Usage:")
	fmt.Println(`  curl -X POST http://localhost:8080/api/print \`)
	fmt.Println(`    -H "Content-Type: application/json" \`)
	fmt.Println(`    -d '{"text":"# Hello World!\nThis is **bold** text."}'`)
}
