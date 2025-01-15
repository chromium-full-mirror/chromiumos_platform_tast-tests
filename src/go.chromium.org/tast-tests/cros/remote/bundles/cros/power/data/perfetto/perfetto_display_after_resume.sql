-- Query the duration from the end of kernel resume to the Chrome showing display in milisseconds.

-- The kernel_resume table will store the last time the kernel finished starting a process as 'end'.
WITH kernel_resume AS (SELECT
	MAX(ts) AS end
	FROM slices
	WHERE name = 'thaw_processes(0)')
SELECT -- And this queries the time when the powerd shows 'Chrome is using * display mode' in the log.
	(MIN(ts) - end)/1000000 AS display_after_resume_ms -- Then the difference of them is the duration.
FROM android_logs LEFT JOIN thread AS t USING(utid) LEFT JOIN process AS p USING(upid) JOIN kernel_resume
WHERE ts > end AND p.name = '/usr/bin/powerd' AND msg LIKE '%Chrome is using % display mode%'