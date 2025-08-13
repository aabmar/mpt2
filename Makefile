# Makefile for MPT-II Go thermal printer tools

# Add .exe suffix on Windows
ifeq ($(OS),Windows_NT)
	EXE := .exe
endif

BIN_DIR := bin
CLI := $(BIN_DIR)/mptprinter-cli$(EXE)
PRINT := $(BIN_DIR)/mptprint$(EXE)
MARKDOWN := $(BIN_DIR)/mpt-markdown$(EXE)

.PHONY: all clean help


help:
	@echo "Build targets:"
	@echo "  make         # Build all tools (default)"
	@echo "  make clean   # Remove built binaries"
	@echo "  make help    # Show this help message"
	@echo "\nUsage:"
	@echo "  ./bin/mptprint \"Hello, World!\"                    # Simple printing"
	@echo "  ./bin/mptprinter-cli -text \"Hello\" -bold -center  # Advanced printing"
	@echo "  ./bin/mpt-markdown README.md                 # Print Markdown file"
	@echo "\nFor help:"
	@echo "  ./bin/mptprinter-cli -help"

all: $(CLI) $(PRINT) $(MARKDOWN)

$(BIN_DIR):
	@mkdir -p $(BIN_DIR)

$(CLI): $(BIN_DIR)
	@echo "Building mptprinter-cli..."
	go build -o $(CLI) ./cmd/mptprinter-cli
	@echo "✓ mptprinter-cli built successfully"

$(PRINT): $(BIN_DIR)
	@echo "Building mptprint..."
	go build -o $(PRINT) ./cmd/mptprint
	@echo "✓ mptprint built successfully"

$(MARKDOWN): $(BIN_DIR)
	@echo "Building mpt-markdown..."
	go build -o $(MARKDOWN) ./cmd/mpt-markdown
	@echo "✓ mpt-markdown built successfully"

clean:
	rm -rf $(BIN_DIR)
