#!/bin/bash
# Build script for MPT-II Go thermal printer tools

echo "Building MPT-II Go thermal printer tools..."

# Create bin directory
mkdir -p bin

# Build main CLI tool
echo "Building mptprinter-cli..."
go build -o bin/mptprinter-cli ./cmd/mptprinter-cli
if [ $? -eq 0 ]; then
    echo "✓ mptprinter-cli built successfully"
else
    echo "✗ Failed to build mptprinter-cli"
    exit 1
fi

# Build simple print tool
echo "Building mptprint..."
go build -o bin/mptprint ./cmd/mptprint
if [ $? -eq 0 ]; then
    echo "✓ mptprint built successfully"
else
    echo "✗ Failed to build mptprint"
    exit 1
fi

echo ""
echo "Build complete! Tools available in ./bin/"
echo ""
echo "Usage:"
echo "  ./bin/mptprint \"Hello, World!\"                    # Simple printing"
echo "  ./bin/mptprinter-cli -text \"Hello\" -bold -center  # Advanced printing"
echo ""
echo "For help:"
echo "  ./bin/mptprinter-cli -help"
