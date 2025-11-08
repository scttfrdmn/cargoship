#!/usr/bin/env python3
"""
Sync GitHub labels from labels.yml file using gh CLI.
"""

import yaml
import subprocess
import sys
from pathlib import Path

def load_labels(yaml_file):
    """Load labels from YAML file."""
    with open(yaml_file, 'r') as f:
        return yaml.safe_load(f)

def get_existing_labels(repo):
    """Get existing labels from GitHub."""
    try:
        result = subprocess.run(
            ['gh', 'label', 'list', '--repo', repo, '--limit', '1000', '--json', 'name,description,color'],
            capture_output=True,
            text=True,
            check=True
        )
        import json
        return {label['name']: label for label in json.loads(result.stdout)}
    except subprocess.CalledProcessError as e:
        print(f"❌ Error fetching existing labels: {e.stderr}")
        return {}

def create_or_update_label(repo, label, existing_labels):
    """Create or update a label using gh CLI."""
    name = label['name']
    description = label.get('description', '')
    color = label['color']

    if name in existing_labels:
        # Update existing label
        existing = existing_labels[name]
        if existing['description'] == description and existing['color'] == color:
            print(f"⏭️  Label unchanged: {name}")
            return 'unchanged'

        try:
            subprocess.run(
                ['gh', 'label', 'edit', name, '--repo', repo,
                 '--description', description, '--color', color],
                capture_output=True,
                text=True,
                check=True
            )
            print(f"✅ Updated label: {name}")
            return 'updated'
        except subprocess.CalledProcessError as e:
            print(f"❌ Failed to update label {name}: {e.stderr}")
            return 'failed'
    else:
        # Create new label
        try:
            subprocess.run(
                ['gh', 'label', 'create', name, '--repo', repo,
                 '--description', description, '--color', color],
                capture_output=True,
                text=True,
                check=True
            )
            print(f"✅ Created label: {name}")
            return 'created'
        except subprocess.CalledProcessError as e:
            print(f"❌ Failed to create label {name}: {e.stderr}")
            return 'failed'

def main():
    repo = 'scttfrdmn/cargoship'
    labels_file = Path(__file__).parent.parent / '.github' / 'labels.yml'

    if not labels_file.exists():
        print(f"❌ Error: labels.yml not found at {labels_file}")
        sys.exit(1)

    print(f"📋 Loading labels from {labels_file}...")
    labels = load_labels(labels_file)

    if not labels:
        print("❌ Error: No labels found in labels.yml")
        sys.exit(1)

    print(f"✅ Loaded {len(labels)} labels")
    print()

    print(f"🔍 Fetching existing labels from {repo}...")
    existing_labels = get_existing_labels(repo)
    print(f"✅ Found {len(existing_labels)} existing labels")
    print()

    print("🏷️  Syncing labels...")
    print()

    stats = {'created': 0, 'updated': 0, 'unchanged': 0, 'failed': 0}

    for label in labels:
        result = create_or_update_label(repo, label, existing_labels)
        stats[result] += 1

    print()
    print("📊 Summary:")
    print(f"   Created: {stats['created']}")
    print(f"   Updated: {stats['updated']}")
    print(f"   Unchanged: {stats['unchanged']}")
    print(f"   Failed: {stats['failed']}")
    print()

    if stats['failed'] > 0:
        print("⚠️  Some labels failed to sync")
        sys.exit(1)
    else:
        print("✅ All labels synced successfully!")

if __name__ == '__main__':
    main()
