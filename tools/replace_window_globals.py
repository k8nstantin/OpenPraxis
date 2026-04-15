#!/usr/bin/env python3
"""Replace window._* globals with OL.* namespace functions in the dashboard UI.

This script performs the following transformations:
1. window._functionName = ... -> OL.functionName = ...
2. window._functionName(...) -> OL.functionName(...)
3. window._functionName && window._functionName(...) -> OL.functionName && OL.functionName(...)
4. Special handling for _copy (already on OL), _wsListeners, _loadProduct

Usage: python3 tools/replace_window_globals.py [--dry-run]
"""

import os
import re
import sys

UI_DIR = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))),
                      'internal', 'web', 'ui')

# All window._* globals to migrate, mapping old name -> new OL name
# Drop the underscore prefix since OL namespace makes it unnecessary
RENAMES = {
    '_amnesiaAction': 'amnesiaAction',
    '_archiveIdea': 'archiveIdea',
    '_archiveManifest': 'archiveManifest',
    '_archiveMemory': 'archiveMemory',
    '_cancelEditInstructions': 'cancelEditInstructions',
    '_connectAgent': 'connectAgent',
    '_copy': 'copy',  # Already exists as OL.copy — just remove window._copy refs
    '_createProduct': 'createProduct',
    '_deleteVisceral': 'deleteVisceral',
    '_delusionAction': 'delusionAction',
    '_disconnectAgent': 'disconnectAgent',
    '_editInstructions': 'editInstructions',
    '_editProduct': 'editProduct',
    '_emergencyStopAll': 'emergencyStopAll',
    '_goToIdea': 'goToIdea',
    '_killTask': 'killTask',
    '_linkIdeaToProduct': 'linkIdeaToProduct',
    '_linkManifestToProduct': 'linkManifestToProduct',
    '_loadConv': 'loadConv',
    '_loadIdea': 'loadIdea',
    '_loadManifest': 'loadManifest',
    '_loadProduct': 'loadProduct',  # Currently alias: window._loadProduct = OL.loadProductDetail
    '_markerDone': 'markerDone',
    '_markerSeen': 'markerSeen',
    '_pauseTask': 'pauseTask',
    '_promoteToManifest': 'promoteToManifest',
    '_quickSchedule': 'quickSchedule',
    '_removeDependency': 'removeDependency',
    '_rescheduleTask': 'rescheduleTask',
    '_resumeTask': 'resumeTask',
    '_saveInstructions': 'saveInstructions',
    '_scheduleAt': 'scheduleAt',
    '_setDependency': 'setDependency',
    '_showProductDiagram': 'showProductDiagram',
    '_taskAction': 'taskAction',
    '_taskArchive': 'taskArchive',
    '_taskStart': 'taskStart',
    '_unlinkIdeaFromProduct': 'unlinkIdeaFromProduct',
    '_unlinkManifestFromProduct': 'unlinkManifestFromProduct',
    '_updateMaxTurns': 'updateMaxTurns',
    '_updateProductStatus': 'updateProductStatus',
}


def process_file(filepath, dry_run=False):
    """Process a single file, replacing window._* with OL.* references."""
    with open(filepath, 'r') as f:
        content = f.read()

    original = content
    changes = []

    # Special case 1: window._copy = OL.copy = function(text) {
    # -> Just OL.copy = function(text) {  (remove the window._copy = part)
    old = 'window._copy = OL.copy = function(text) {'
    new = 'OL.copy = function(text) {'
    if old in content:
        content = content.replace(old, new)
        changes.append(f'  Removed dual-assign: window._copy = OL.copy -> OL.copy')

    # Special case 2: window._loadProduct = OL.loadProductDetail;
    # -> OL.loadProduct = OL.loadProductDetail;
    old = 'window._loadProduct = OL.loadProductDetail;'
    new = 'OL.loadProduct = OL.loadProductDetail;'
    if old in content:
        content = content.replace(old, new)
        changes.append(f'  Alias: window._loadProduct -> OL.loadProduct')

    # Special case 3: window._wsListeners
    # This is a data array, not a function — keep it on OL namespace
    content = content.replace('window._wsListeners', 'OL._wsListeners')
    if 'OL._wsListeners' in content and 'window._wsListeners' in original:
        changes.append(f'  Renamed: window._wsListeners -> OL._wsListeners')

    # Now handle all function definitions and references
    for old_name, new_name in RENAMES.items():
        # Skip _copy — already handled above
        if old_name == '_copy':
            # Still need to replace remaining window._copy(...) call references
            # Pattern: window._copy( -> OL.copy(
            pattern = r'window\._copy\('
            replacement = 'OL.copy('
            if re.search(pattern, content):
                count = len(re.findall(pattern, content))
                content = re.sub(pattern, replacement, content)
                changes.append(f'  Refs: window._copy( -> OL.copy( [{count}x]')
            continue

        if old_name == '_loadProduct':
            # Already handled the definition alias above
            # Handle remaining references: window._loadProduct(...)
            pattern = r'window\._loadProduct\b'
            replacement = 'OL.loadProduct'
            if re.search(pattern, content):
                count = len(re.findall(pattern, content))
                content = re.sub(pattern, replacement, content)
                changes.append(f'  Refs: window._loadProduct -> OL.loadProduct [{count}x]')
            continue

        # Replace definitions: window._name = function/async function
        # Pattern: window._oldName = (async )?function
        def_pattern = rf'window\.{re.escape(old_name)}\s*='
        def_replacement = f'OL.{new_name} ='
        if re.search(def_pattern, content):
            count = len(re.findall(def_pattern, content))
            content = re.sub(def_pattern, def_replacement, content)
            changes.append(f'  Def: window.{old_name} -> OL.{new_name} [{count}x]')

        # Replace call references: window._oldName(
        ref_pattern = rf'window\.{re.escape(old_name)}\('
        ref_replacement = f'OL.{new_name}('
        if re.search(ref_pattern, content):
            count = len(re.findall(ref_pattern, content))
            content = re.sub(ref_pattern, ref_replacement, content)
            changes.append(f'  Call: window.{old_name}( -> OL.{new_name}( [{count}x]')

        # Replace guarded references: window._oldName&&window._oldName(
        # Already caught by the above patterns

        # Replace bare references (e.g. btn.onclick = window._emergencyStopAll)
        bare_pattern = rf'window\.{re.escape(old_name)}\b(?!\()'
        bare_replacement = f'OL.{new_name}'
        if re.search(bare_pattern, content):
            count = len(re.findall(bare_pattern, content))
            content = re.sub(bare_pattern, bare_replacement, content)
            changes.append(f'  Bare: window.{old_name} -> OL.{new_name} [{count}x]')

    if content != original:
        rel = os.path.relpath(filepath, UI_DIR)
        print(f'\n{rel}: {len(changes)} change(s)')
        for c in changes:
            print(c)

        if not dry_run:
            with open(filepath, 'w') as f:
                f.write(content)
            print(f'  -> Written')
        else:
            print(f'  -> [dry-run] Not written')
        return True

    return False


def main():
    dry_run = '--dry-run' in sys.argv

    if dry_run:
        print('=== DRY RUN MODE ===\n')

    files_changed = 0

    # Process all JS files and HTML files
    for root, dirs, files in os.walk(UI_DIR):
        for fname in sorted(files):
            if fname.endswith(('.js', '.html')):
                fpath = os.path.join(root, fname)
                if process_file(fpath, dry_run):
                    files_changed += 1

    print(f'\n{"=" * 40}')
    print(f'Files changed: {files_changed}')

    # Verify no remaining window._ references (except OL._switchLifecycle, OL._activeLifecycleView)
    print('\nRemaining window._ references:')
    remaining = 0
    for root, dirs, files in os.walk(UI_DIR):
        for fname in sorted(files):
            if fname.endswith(('.js', '.html')):
                fpath = os.path.join(root, fname)
                with open(fpath, 'r') as f:
                    for i, line in enumerate(f, 1):
                        # Look for window._ but not OL._ (which is fine)
                        if 'window._' in line:
                            rel = os.path.relpath(fpath, UI_DIR)
                            print(f'  {rel}:{i}: {line.strip()}')
                            remaining += 1

    if remaining:
        print(f'\nWARNING: {remaining} remaining window._ reference(s)')
    else:
        print('  None — all clean!')


if __name__ == '__main__':
    main()
