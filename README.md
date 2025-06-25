
## Prerequisites

- Go 1.22.3 or higher
- Windows/Linux/macOS support

## Build Instructions

### Clone and Build

```bash
# Navigate to the project directory
cd project_dir

# Build the executable
go build -o godb.exe btree.go kvstore_ultra.go ultra_cli.go

# Or use go build with all files
go build -o godb.exe *.go
```

### Verify Build

```bash
# Check if executable was created
dir godb.exe

# Run the executable
.\godb.exe
```

## Running the Application

### Interactive CLI Mode

```bash
# Start the interactive CLI
.\godb.exe

# Available commands in CLI:
# SET key value    - Store a key-value pair
# GET key         - Retrieve value for a key
# DEL key         - Delete a key-value pair
# LIST            - List all keys
# STATS           - Show database statistics
# EXIT            - Exit the application
```

### Example Usage

```bash
.\godb.exe

> SET user:1 "John Doe"
OK

> SET user:2 "Jane Smith"
OK

> GET user:1
"John Doe"

> LIST
user:1
user:2

> STATS
Keys: 2
Cache Hit Rate: 100%

> EXIT
```

## License

This project is open source with MIT LICENSE file for details.

