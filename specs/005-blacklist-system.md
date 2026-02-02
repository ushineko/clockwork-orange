# Spec 005: Shared Blacklist and Image Review System

**Status**: COMPLETE
**Implementation Date**: 2024-07
**Commit**: (baseline feature)

## Overview
Implement a centralized blacklist system shared across all plugins with GUI-based image review functionality to allow users to ban unwanted wallpapers.

## Requirements

### Functional Requirements
1. Centralized blacklist database shared by all plugins
2. Hash-based image identification (survives renames/moves)
3. GUI review mode for scanning and marking images
4. Permanent deletion + blacklist on user action
5. Blacklist manager UI for viewing and removing entries

### Technical Requirements
1. Use perceptual hashing or SHA256 for image identification
2. Store blacklist in JSON format (`~/.config/clockwork-orange-blacklist.json`)
3. Include metadata: file hash, deletion date, source plugin, original filename
4. Filter blacklisted images from all plugin results
5. Support both automatic and manual blacklist additions

## Implementation Details

### Blacklist Database Structure
```json
{
  "blacklist": [
    {
      "hash": "abc123...",
      "deleted_at": "2024-07-15T10:30:00",
      "source_plugin": "google_images",
      "original_filename": "cyberpunk-city.jpg"
    }
  ]
}
```

### Blacklist Module (`plugins/blacklist.py`)
- `BlacklistManager` class
- `add_to_blacklist(file_path)` - compute hash and add entry
- `is_blacklisted(file_path)` - check if hash exists in blacklist
- `remove_from_blacklist(hash)` - remove entry (user correction)
- `get_all_entries()` - return all blacklist entries for UI display

### Review Mode (GUI Integration)
Located in `plugins_tab.py`:
1. **Scan Button**: Load all images from plugin download directory
2. **Image Preview**: Display current image in preview panel
3. **Navigation**: Left/Right arrow keys to browse
4. **Delete Button**: Mark image for deletion and blacklisting
5. **Confirmation**: Delete file from disk + add hash to blacklist

### Blacklist Manager Tab (GUI)
- Display table of all blacklisted entries
- Show hash (truncated), deletion date, source plugin, filename
- "Remove from Blacklist" button to undo mistakes
- Filter/search functionality

### Plugin Integration
Each plugin filters results through blacklist before returning:
```python
def get_images(self):
    all_images = self._scan_directory()
    return [img for img in all_images if not blacklist.is_blacklisted(img)]
```

## Acceptance Criteria

- [x] `BlacklistManager` class implemented in `plugins/blacklist.py`
- [x] Blacklist database persists in JSON format
- [x] All plugins filter results through blacklist
- [x] Review mode loads images from plugin directories
- [x] Delete button in review mode adds to blacklist and removes file
- [x] Blacklist Manager tab displays all entries
- [x] Remove from blacklist functionality works
- [x] Hash-based identification survives file renames
- [x] Blacklist entries include metadata (date, source, filename)

## Testing

### Test Cases
1. Add image to blacklist via review mode
2. Verify image no longer appears in plugin results
3. Rename blacklisted image and verify still blocked
4. Remove entry from blacklist manager and verify image reappears
5. Blacklist same image from different plugins (deduplication)
6. Review mode with empty directory (graceful handling)
7. Review mode with 1000+ images (performance)

### Expected Behavior
- Blacklisted images never appear as wallpaper candidates
- Deletion is permanent (file removed from disk)
- Blacklist survives application restarts
- Multiple plugins share the same blacklist database
- Review mode is responsive even with large image collections

## Notes
- Uses SHA256 hashing (could upgrade to perceptual hashing for near-duplicates)
- Blacklist applies to all plugins uniformly
- No way to "preview before blacklist" - deletion is immediate
- Future: Add "quarantine" mode (move to folder instead of delete)
- Future: Perceptual hashing to catch similar images
- Future: Export/import blacklist for sharing across machines
