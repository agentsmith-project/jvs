#!/usr/bin/env bash
# jvs-sync - Helper script for syncing JVS repositories between machines
#
# This script provides convenient backup, mirror, and migration operations
# for JVS repositories while properly handling .jvs/ metadata.
#
# Usage: jvs-sync [command] [options] <source> <destination>
#
# Commands:
#   backup   - Backup repository to remote location
#   restore  - Restore repository from remote location
#   mirror   - Bidirectional sync between two locations
#   migrate  - Migrate repository to new location
#   verify   - Verify sync integrity

set -euo pipefail

# Colors for output
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly NC='\033[0m' # No Color

# Default settings
DRY_RUN=false
VERBOSE=false
THREADS=10
EXCLUDE_PATTERNS=()
RSYNC_ONLY=false
DEFAULT_JVS_EXCLUDE=(".jvs/locks/**" ".jvs/intents/**" ".jvs/gc/*.json" ".jvs/index.sqlite")
JVS_EXCLUDE=("${DEFAULT_JVS_EXCLUDE[@]}")

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $*"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*" >&2
}

log_verbose() {
    if [[ "$VERBOSE" == true ]]; then
        echo -e "[VERBOSE] $*"
    fi
}

# Show usage
show_usage() {
    cat << EOF
${BLUE}JVS Sync Helper${NC} - Backup and sync JVS repositories between machines

${YELLOW}USAGE:${NC}
    jvs-sync [command] [options] <source> <destination>

${YELLOW}COMMANDS:${NC}
    backup    Backup repository to remote location
    restore   Restore repository from remote location
    mirror    Bidirectional sync between two locations
    migrate   Migrate repository to new location (one-way)
    verify    Verify sync integrity between locations

${YELLOW}OPTIONS:${NC}
    -n, --dry-run       Show what would be done without making changes
    -v, --verbose       Show detailed output
    -j, --threads N     Number of concurrent threads (default: 10)
    -e, --exclude PATTERN  Exclude pattern (can be repeated)
    --rsync-only        Force use of rsync even if juicefs available

${YELLOW}EXAMPLES:${NC}
    # Backup to remote server via SSH
    jvs-sync backup /path/to/repo user@server:/backup/jvs

    # Backup to S3 via JuiceFS sync
    jvs-sync backup /path/to/repo s3://mybucket/jvs-backup

    # Restore from backup
    jvs-sync restore user@server:/backup/jvs /path/to/repo

    # Mirror between two local directories
    jvs-sync mirror /path/to/repo /path/to/mirror

    # Migrate to new storage
    jvs-sync migrate /old/location /new/location

    # Verify backup integrity
    jvs-sync verify /path/to/repo user@server:/backup/jvs

${YELLOW}WHAT GETS SYNCED:${NC}
    - .jvs/ format_version, worktrees/, snapshots/, descriptors/
    - .jvs/ audit/ and committed gc/tombstones/ metadata
    - main/ and worktrees/*/ payload directories

${YELLOW}WHAT GETS EXCLUDED:${NC}
    - .jvs/locks/** (host-local mutation locks, not portable)
    - .jvs/intents/** (in-flight operations, not portable)
    - .jvs/gc/*.json (active GC plans, not portable)
    - .jvs/index.sqlite (rebuildable cache)

EOF
}

# Check if source is a valid JVS repository
check_jvs_repo() {
    local repo="$1"

    if [[ ! -d "$repo/.jvs" ]]; then
        log_error "Not a JVS repository: $repo"
        log_error "Missing .jvs/ directory"
        return 1
    fi

    if [[ ! -f "$repo/.jvs/format_version" ]]; then
        log_error "Invalid JVS repository: $repo"
        log_error "Missing .jvs/format_version file"
        return 1
    fi

    return 0
}

# Detect sync method (juicefs or rsync)
detect_sync_method() {
    local source="$1"
    local dest="$2"
    local force_rsync="${3:-false}"

    if [[ "$force_rsync" == true ]]; then
        echo "rsync"
        return
    fi

    # Check if juicefs sync is available and destination supports it
    if command -v juicefs &> /dev/null; then
        # Check if source or dest looks like an object storage URL
        if [[ "$source" =~ ^[s3|oss|gs|cos|az|juicefs]:// ]] || \
           [[ "$dest" =~ ^[s3|oss|gs|cos|az|juicefs]:// ]]; then
            echo "juicefs"
            return
        fi
    fi

    echo "rsync"
}

# Build exclude patterns for rsync
build_rsync_excludes() {
    local excludes=()
    excludes+=("${JVS_EXCLUDE[@]}")
    excludes+=("${EXCLUDE_PATTERNS[@]}")

    for pattern in "${excludes[@]}"; do
        printf -- '--exclude=%s ' "$pattern"
    done
}

# Build exclude patterns for juicefs
build_juicefs_excludes() {
    local excludes=()
    excludes+=("${JVS_EXCLUDE[@]}")
    excludes+=("${EXCLUDE_PATTERNS[@]}")

    for pattern in "${excludes[@]}"; do
        printf -- '--exclude=%s ' "$pattern"
    done
}

# Sync using rsync
rsync_sync() {
    local source="$1"
    local dest="$2"
    local delete="${3:-false}"
    local update="${4:-true}"

    local rsync_opts=(
        -a                          # archive mode
        -v                          # verbose
        --human-readable
        --progress
    )

    if [[ "$VERBOSE" == true ]]; then
        rsync_opts+=(-vv)
    fi

    if [[ "$DRY_RUN" == true ]]; then
        rsync_opts+=(--dry-run)
    fi

    if [[ "$delete" == true ]]; then
        rsync_opts+=(--delete)
        rsync_opts+=(--delete-after)
    fi

    if [[ "$update" == true ]]; then
        rsync_opts+=(-u)  # skip files newer on destination
    fi

    # Add excludes
    local pattern
    for pattern in "${JVS_EXCLUDE[@]}" "${EXCLUDE_PATTERNS[@]}"; do
        rsync_opts+=(--exclude="$pattern")
    done

    # Run rsync
    log_info "Running rsync from $source to $dest"
    if [[ "$DRY_RUN" == true ]]; then
        log_warning "DRY RUN - no changes will be made"
    fi

    rsync "${rsync_opts[@]}" "$source/" "$dest/"
}

# Sync using juicefs
juicefs_sync() {
    local source="$1"
    local dest="$2"
    local delete_src="${3:-false}"
    local delete_dst="${4:-false}"

    local juicefs_opts=(
        --threads "$THREADS"
        --perms
        --links
    )

    if [[ "$VERBOSE" == true ]]; then
        juicefs_opts+=(--verbose)
    fi

    if [[ "$DRY_RUN" == true ]]; then
        juicefs_opts+=(--dry)
        log_warning "DRY RUN - no changes will be made"
    fi

    if [[ "$delete_src" == true ]]; then
        juicefs_opts+=(--delete-src)
    fi

    if [[ "$delete_dst" == true ]]; then
        juicefs_opts+=(--delete-dst)
    fi

    # Add excludes
    local pattern
    for pattern in "${JVS_EXCLUDE[@]}" "${EXCLUDE_PATTERNS[@]}"; do
        juicefs_opts+=(--exclude="$pattern")
    done

    # Run juicefs sync
    log_info "Running juicefs sync from $source to $dest"
    juicefs sync "${juicefs_opts[@]}" "$source" "$dest"
}

# Backup command
cmd_backup() {
    local source="$1"
    local dest="$2"

    log_info "Starting JVS repository backup..."
    log_verbose "Source: $source"
    log_verbose "Destination: $dest"

    if ! check_jvs_repo "$source"; then
        return 1
    fi

    local method
    method=$(detect_sync_method "$source" "$dest" "$RSYNC_ONLY")

    case "$method" in
        juicefs)
            juicefs_sync "$source" "$dest" false true
            ;;
        rsync)
            rsync_sync "$source" "$dest" false true
            ;;
    esac

    log_success "Backup completed successfully"
}

# Restore command
cmd_restore() {
    local source="$1"
    local dest="$2"

    log_info "Starting JVS repository restore..."
    log_verbose "Source: $source"
    log_verbose "Destination: $dest"

    # Create destination if it doesn't exist
    if [[ ! -d "$dest" ]]; then
        log_info "Creating destination directory: $dest"
        if [[ "$DRY_RUN" == false ]]; then
            mkdir -p "$dest"
        fi
    fi

    local method
    method=$(detect_sync_method "$source" "$dest" "$RSYNC_ONLY")

    case "$method" in
        juicefs)
            juicefs_sync "$source" "$dest" false false
            ;;
        rsync)
            # For restore, don't use -u (update) to ensure all files are copied
            rsync_sync "$source" "$dest" false false
            ;;
    esac

    log_success "Restore completed successfully"
}

# Mirror command (bidirectional sync)
cmd_mirror() {
    local source="$1"
    local dest="$2"

    log_info "Starting bidirectional mirror sync..."
    log_warning "This will sync changes in BOTH directions"

    if ! check_jvs_repo "$source"; then
        return 1
    fi

    local method
    method=$(detect_sync_method "$source" "$dest" "$RSYNC_ONLY")

    if [[ "$method" == "juicefs" ]]; then
        log_error "Bidirectional sync not supported with juicefs sync"
        log_error "Use 'backup' or 'migrate' instead"
        return 1
    fi

    # First pass: source -> dest
    log_info "Syncing from source to destination..."
    rsync_sync "$source" "$dest" false false

    # Second pass: dest -> source
    log_info "Syncing from destination to source..."
    rsync_sync "$dest" "$source" false false

    log_success "Mirror sync completed successfully"
}

# Migrate command (one-way with delete)
cmd_migrate() {
    local source="$1"
    local dest="$2"

    log_warning "Starting JVS repository migration..."
    log_warning "This will delete extraneous files in the destination"
    log_verbose "Source: $source"
    log_verbose "Destination: $dest"

    if ! check_jvs_repo "$source"; then
        return 1
    fi

    # Create destination if it doesn't exist
    if [[ ! -d "$dest" ]]; then
        log_info "Creating destination directory: $dest"
        if [[ "$DRY_RUN" == false ]]; then
            mkdir -p "$dest"
        fi
    fi

    local method
    method=$(detect_sync_method "$source" "$dest" "$RSYNC_ONLY")

    case "$method" in
        juicefs)
            juicefs_sync "$source" "$dest" false true
            ;;
        rsync)
            rsync_sync "$source" "$dest" true false
            ;;
    esac

    log_success "Migration completed successfully"
    log_info "Source repository remains intact"
    log_warning "Verify migration before deleting source"
}

# Verify command
cmd_verify() {
    local source="$1"
    local dest="$2"

    log_info "Verifying sync integrity..."

    if ! check_jvs_repo "$source"; then
        return 1
    fi

    # Check if destination is accessible
    if [[ ! -d "$dest" && ! "$dest" =~ ^[a-z]+:// ]]; then
        log_error "Destination not accessible: $dest"
        return 1
    fi

    local has_errors=false

    # Verify .jvs/format_version
    log_info "Checking format_version..."
    local src_fmt
    local dst_fmt

    if [[ -f "$source/.jvs/format_version" ]]; then
        src_fmt=$(cat "$source/.jvs/format_version")
    fi

    if [[ -d "$dest/.jvs" && -f "$dest/.jvs/format_version" ]]; then
        dst_fmt=$(cat "$dest/.jvs/format_version")
    elif [[ "$dest" =~ ^[a-z]+:// ]]; then
        log_info "Skipping format_version check for object storage"
    else
        log_error "Destination format_version not found"
        has_errors=true
    fi

    if [[ -n "$src_fmt" && -n "$dst_fmt" ]]; then
        if [[ "$src_fmt" == "$dst_fmt" ]]; then
            log_success "Format versions match: $src_fmt"
        else
            log_error "Format version mismatch: source=$src_fmt, dest=$dst_fmt"
            has_errors=true
        fi
    fi

    # Count snapshots
    log_info "Checking snapshot counts..."
    local src_snapshots
    local dst_snapshots

    if [[ -d "$source/.jvs/snapshots" ]]; then
        src_snapshots=$(find "$source/.jvs/snapshots" -mindepth 1 -maxdepth 1 -type d | wc -l)
        log_info "Source snapshots: $src_snapshots"
    fi

    if [[ -d "$dest/.jvs/snapshots" ]]; then
        dst_snapshots=$(find "$dest/.jvs/snapshots" -mindepth 1 -maxdepth 1 -type d | wc -l)
        log_info "Destination snapshots: $dst_snapshots"
    fi

    if [[ -n "$src_snapshots" && -n "$dst_snapshots" ]]; then
        if [[ "$src_snapshots" -eq "$dst_snapshots" ]]; then
            log_success "Snapshot counts match"
        else
            log_warning "Snapshot count mismatch: source=$src_snapshots, dest=$dst_snapshots"
        fi
    fi

    # Count descriptors
    log_info "Checking descriptor counts..."
    local src_descriptors
    local dst_descriptors

    if [[ -d "$source/.jvs/descriptors" ]]; then
        src_descriptors=$(find "$source/.jvs/descriptors" -name "*.json" | wc -l)
        log_info "Source descriptors: $src_descriptors"
    fi

    if [[ -d "$dest/.jvs/descriptors" ]]; then
        dst_descriptors=$(find "$dest/.jvs/descriptors" -name "*.json" | wc -l)
        log_info "Destination descriptors: $dst_descriptors"
    fi

    if [[ -n "$src_descriptors" && -n "$dst_descriptors" ]]; then
        if [[ "$src_descriptors" -eq "$dst_descriptors" ]]; then
            log_success "Descriptor counts match"
        else
            log_warning "Descriptor count mismatch: source=$src_descriptors, dest=$dst_descriptors"
        fi
    fi

    if [[ "$has_errors" == false ]]; then
        log_success "Verification passed"
        return 0
    else
        log_error "Verification failed"
        return 1
    fi
}

# Main script
main() {
    # Check for help flag anywhere in arguments
    for arg in "$@"; do
        if [[ "$arg" == "-h" || "$arg" == "--help" ]]; then
            show_usage
            exit 0
        fi
    done

    if [[ $# -eq 0 ]]; then
        show_usage
        exit 0
    fi

    local command="$1"
    shift

    # Parse options (excluding -h/--help which was handled above)
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -n|--dry-run)
                DRY_RUN=true
                shift
                ;;
            -v|--verbose)
                VERBOSE=true
                shift
                ;;
            -j|--threads)
                THREADS="$2"
                shift 2
                ;;
            -e|--exclude)
                EXCLUDE_PATTERNS+=("$2")
                shift 2
                ;;
            --rsync-only)
                RSYNC_ONLY=true
                shift
                ;;
            --no-intents)
                JVS_EXCLUDE=("${DEFAULT_JVS_EXCLUDE[@]}")
                shift
                ;;
            -*)
                log_error "Unknown option: $1"
                show_usage
                exit 1
                ;;
            *)
                break
                ;;
        esac
    done

    # Check remaining arguments
    if [[ $# -lt 2 ]]; then
        log_error "Missing source or destination argument"
        show_usage
        exit 1
    fi

    local source="$1"
    local dest="$2"

    # Execute command
    case "$command" in
        backup)
            cmd_backup "$source" "$dest"
            ;;
        restore)
            cmd_restore "$source" "$dest"
            ;;
        mirror)
            cmd_mirror "$source" "$dest"
            ;;
        migrate)
            cmd_migrate "$source" "$dest"
            ;;
        verify)
            cmd_verify "$source" "$dest"
            ;;
        *)
            log_error "Unknown command: $command"
            show_usage
            exit 1
            ;;
    esac
}

main "$@"
