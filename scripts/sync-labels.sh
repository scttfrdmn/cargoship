#!/bin/bash
# Sync GitHub labels from labels.yml file using gh CLI
# Pure bash implementation - no external dependencies

set -e

REPO="scttfrdmn/cargoship"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LABELS_FILE="$SCRIPT_DIR/../.github/labels.yml"

if [[ ! -f "$LABELS_FILE" ]]; then
    echo "❌ Error: labels.yml not found at $LABELS_FILE"
    exit 1
fi

echo "🏷️  Syncing labels from $LABELS_FILE"
echo ""

# Parse YAML and extract label information
# This is a simple parser that assumes the YAML structure:
# - name: "label name"
#   description: "label description"
#   color: "hex color"

created=0
updated=0
unchanged=0
failed=0

# Read the YAML file and process each label
current_name=""
current_desc=""
current_color=""

while IFS= read -r line; do
    # Skip comments and empty lines
    [[ "$line" =~ ^[[:space:]]*# ]] && continue
    [[ -z "$(echo "$line" | tr -d '[:space:]')" ]] && continue

    # Detect start of new label entry
    if [[ "$line" =~ ^-[[:space:]]*name:[[:space:]]*\"(.*)\"$ ]] || \
       [[ "$line" =~ ^-[[:space:]]*name:[[:space:]]*\'(.*)\'$ ]] || \
       [[ "$line" =~ ^-[[:space:]]*name:[[:space:]]*(.*)$ ]]; then
        # Process previous label if we have one
        if [[ -n "$current_name" ]]; then
            # Try to create the label
            if gh label list --repo "$REPO" --limit 1000 | grep -q "^$current_name"; then
                # Label exists, update it
                if gh label edit "$current_name" --repo "$REPO" \
                    --description "$current_desc" \
                    --color "$current_color" &>/dev/null; then
                    echo "✅ Updated: $current_name"
                    ((updated++))
                else
                    echo "⚠️  Unchanged: $current_name (already up to date)"
                    ((unchanged++))
                fi
            else
                # Label doesn't exist, create it
                if gh label create "$current_name" --repo "$REPO" \
                    --description "$current_desc" \
                    --color "$current_color" &>/dev/null; then
                    echo "✅ Created: $current_name"
                    ((created++))
                else
                    echo "❌ Failed: $current_name"
                    ((failed++))
                fi
            fi
        fi

        # Extract new label name
        current_name="${BASH_REMATCH[1]}"
        # Remove quotes if present
        current_name=$(echo "$current_name" | sed -e 's/^"//' -e 's/"$//' -e "s/^'//" -e "s/'$//")
        current_desc=""
        current_color=""

    # Extract description
    elif [[ "$line" =~ ^[[:space:]]+description:[[:space:]]*\"(.*)\"$ ]] || \
         [[ "$line" =~ ^[[:space:]]+description:[[:space:]]*\'(.*)\'$ ]] || \
         [[ "$line" =~ ^[[:space:]]+description:[[:space:]]*(.*)$ ]]; then
        current_desc="${BASH_REMATCH[1]}"
        # Remove quotes if present
        current_desc=$(echo "$current_desc" | sed -e 's/^"//' -e 's/"$//' -e "s/^'//" -e "s/'$//")

    # Extract color
    elif [[ "$line" =~ ^[[:space:]]+color:[[:space:]]*\"(.*)\"$ ]] || \
         [[ "$line" =~ ^[[:space:]]+color:[[:space:]]*\'(.*)\'$ ]] || \
         [[ "$line" =~ ^[[:space:]]+color:[[:space:]]*(.*)$ ]]; then
        current_color="${BASH_REMATCH[1]}"
        # Remove quotes and # prefix if present
        current_color=$(echo "$current_color" | sed -e 's/^"//' -e 's/"$//' -e "s/^'//" -e "s/'$//" -e 's/^#//')
    fi
done < "$LABELS_FILE"

# Process the last label
if [[ -n "$current_name" ]]; then
    if gh label list --repo "$REPO" --limit 1000 | grep -q "^$current_name"; then
        # Label exists, try to update it
        if gh label edit "$current_name" --repo "$REPO" \
            --description "$current_desc" \
            --color "$current_color" &>/dev/null; then
            echo "✅ Updated: $current_name"
            ((updated++))
        else
            echo "⚠️  Unchanged: $current_name (already up to date)"
            ((unchanged++))
        fi
    else
        # Label doesn't exist, create it
        if gh label create "$current_name" --repo "$REPO" \
            --description "$current_desc" \
            --color "$current_color" &>/dev/null; then
            echo "✅ Created: $current_name"
            ((created++))
        else
            echo "❌ Failed: $current_name"
            ((failed++))
        fi
    fi
fi

echo ""
echo "📊 Summary:"
echo "   Created: $created"
echo "   Updated: $updated"
echo "   Unchanged: $unchanged"
echo "   Failed: $failed"
echo ""

total=$((created + updated + unchanged + failed))
echo "✅ Processed $total labels"

if [[ $failed -gt 0 ]]; then
    echo "⚠️  Some labels failed to sync"
    exit 1
fi
