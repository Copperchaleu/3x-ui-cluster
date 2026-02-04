#!/bin/bash

# Script to clean up Master node (SlaveId=0) configurations
# WARNING: This will permanently delete all inbounds, outbounds, and routing rules
# assigned to the Master node. Make sure you have a backup!

set -e

DB_PATH="${DB_PATH:-/etc/x-ui/x-ui.db}"

echo "=========================================="
echo "3x-ui Master Config Cleanup Script"
echo "=========================================="
echo ""
echo "This script will delete all configurations assigned to Master node (slave_id=0):"
echo "  - Inbounds"
echo "  - Xray Outbounds"
echo "  - Xray Routing Rules"
echo ""
echo "Database: $DB_PATH"
echo ""

# Check if database exists
if [ ! -f "$DB_PATH" ]; then
    echo "❌ Error: Database file not found at $DB_PATH"
    echo "Set DB_PATH environment variable to specify custom location."
    exit 1
fi

# Backup database
BACKUP_PATH="${DB_PATH}.backup.$(date +%Y%m%d_%H%M%S)"
echo "📦 Creating backup: $BACKUP_PATH"
cp "$DB_PATH" "$BACKUP_PATH"
echo "✅ Backup created successfully"
echo ""

# Check current counts
echo "📊 Checking current configuration counts..."
INBOUND_COUNT=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM inbounds WHERE slave_id=0;")
OUTBOUND_COUNT=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM xray_outbounds WHERE slave_id=0;")
ROUTING_COUNT=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM xray_routing_rules WHERE slave_id=0;")

echo "  Inbounds with slave_id=0: $INBOUND_COUNT"
echo "  Outbounds with slave_id=0: $OUTBOUND_COUNT"
echo "  Routing Rules with slave_id=0: $ROUTING_COUNT"
echo ""

TOTAL=$((INBOUND_COUNT + OUTBOUND_COUNT + ROUTING_COUNT))

if [ "$TOTAL" -eq 0 ]; then
    echo "✅ No Master configurations found. Nothing to clean up."
    exit 0
fi

# Confirm deletion
echo "⚠️  WARNING: This will delete $TOTAL configuration(s)!"
echo ""
read -p "Type 'yes' to confirm deletion: " CONFIRM

if [ "$CONFIRM" != "yes" ]; then
    echo "❌ Cleanup cancelled."
    exit 0
fi

echo ""
echo "🗑️  Deleting Master configurations..."

# Execute deletions
sqlite3 "$DB_PATH" "DELETE FROM inbounds WHERE slave_id=0;"
echo "  ✅ Deleted $INBOUND_COUNT inbound(s)"

sqlite3 "$DB_PATH" "DELETE FROM xray_outbounds WHERE slave_id=0;"
echo "  ✅ Deleted $OUTBOUND_COUNT outbound(s)"

sqlite3 "$DB_PATH" "DELETE FROM xray_routing_rules WHERE slave_id=0;"
echo "  ✅ Deleted $ROUTING_COUNT routing rule(s)"

echo ""
echo "✅ Cleanup completed successfully!"
echo ""
echo "📝 Next steps:"
echo "  1. Add Slave server(s) via the web panel"
echo "  2. Recreate your configurations and assign them to Slaves"
echo "  3. If you need to restore, use: cp $BACKUP_PATH $DB_PATH"
echo ""
