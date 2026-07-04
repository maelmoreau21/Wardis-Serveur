package video

// ReconcileRecording calculates the adjustments needed for an incoming recording segment 'rec'
// against a list of existing overlapping recordings.
// It returns:
// - adjustedRec: the modified recording metadata (if any)
// - idsToDelete: list of database IDs of existing recordings to delete (since they are fully covered/redundant)
// - shouldInsert: false if the incoming recording has been made obsolete/redundant
func ReconcileRecording(rec VideoRecording, existing []VideoRecording) (adjustedRec VideoRecording, idsToDelete []string, shouldInsert bool) {
	adjustedRec = rec
	shouldInsert = true

	for _, ext := range existing {
		// Case B: Existing segment completely covers new segment
		if (ext.StartTime.Before(adjustedRec.StartTime) || ext.StartTime.Equal(adjustedRec.StartTime)) &&
			(ext.EndTime.After(adjustedRec.EndTime) || ext.EndTime.Equal(adjustedRec.EndTime)) {
			shouldInsert = false
			return
		}

		// Case A: New segment completely covers existing segment strictly inside
		if adjustedRec.StartTime.Before(ext.StartTime) && adjustedRec.EndTime.After(ext.EndTime) {
			idsToDelete = append(idsToDelete, ext.ID)
			continue
		}

		// Case C: New segment starts before, but ends inside existing segment
		if adjustedRec.StartTime.Before(ext.StartTime) &&
			adjustedRec.EndTime.After(ext.StartTime) &&
			(adjustedRec.EndTime.Before(ext.EndTime) || adjustedRec.EndTime.Equal(ext.EndTime)) {
			adjustedRec.EndTime = ext.StartTime
		}

		// Case D: New segment starts inside, but ends after existing segment
		if (adjustedRec.StartTime.After(ext.StartTime) || adjustedRec.StartTime.Equal(ext.StartTime)) &&
			adjustedRec.StartTime.Before(ext.EndTime) &&
			adjustedRec.EndTime.After(ext.EndTime) {
			adjustedRec.StartTime = ext.EndTime
		}

		// If at any point the adjusted segment is empty or inverted, skip insertion
		if !adjustedRec.StartTime.Before(adjustedRec.EndTime) {
			shouldInsert = false
			return
		}
	}

	return
}
