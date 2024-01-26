package oracle

import (
	"errors"
	"fmt"
	"github.com/rs/zerolog"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// needs the const if imported, cannot os.ReadFile in this case
const defaultMetricsConst = `
[[metric]]
context = "sessions"
labels = ["status", "type"]
cloudwatchtype = { value = "Count" }
metricsdesc = { value = "Gauge metric with count of sessions by status and type." }
request = "SELECT status, type, COUNT(*) as value FROM v$session GROUP BY status, type"

[[metric]]
context = "resource"
labels = ["resource_name"]
metricsdesc = { current_utilization = "Generic counter metric from v$resource_limit view in Oracle (current value).", limit_value = "Generic counter metric from v$resource_limit view in Oracle (UNLIMITED: -1)." }
cloudwatchtype = { current_utilization = "Count", limit_value = "Count" }
request = "SELECT resource_name,current_utilization,CASE WHEN TRIM(limit_value) LIKE 'UNLIMITED' THEN '-1' ELSE TRIM(limit_value) END as limit_value FROM v$resource_limit"

[[metric]]
context = "asm_diskgroup"
labels = ["name"]
metricsdesc = { total = "Total size of ASM disk group.", free = "Free space available on ASM disk group." }
request = "SELECT name,total_mb*1024*1024 as total,free_mb*1024*1024 as free FROM v$asm_diskgroup_stat where exists (select 1 from v$datafile where name like '+%')"
cloudwatchtype = { total = "Bytes", free = "Bytes" }
ignorezeroresult = true

[[metric]]
context = "activity"
metricsdesc = { value = "Generic counter metric from v$sysstat view in Oracle." }
fieldtoappend = "name"
cloudwatchtype = { value = "Count" }
request = "SELECT name, value FROM v$sysstat WHERE name IN ('parse count (total)', 'execute count', 'user commits', 'user rollbacks')"

[[metric]]
context = "process"
metricsdesc = { count = "Gauge metric with count of processes." }
cloudwatchtype = { count = "Count" }
request = "SELECT COUNT(*) as count FROM v$process"

[[metric]]
context = "wait_time"
metricsdesc = { value = "Generic counter metric from v$waitclassmetric view in Oracle." }
fieldtoappend = "wait_class"
cloudwatchtype = { value = "Seconds" }
request = '''
SELECT
  n.wait_class as WAIT_CLASS,
  round(m.time_waited/m.INTSIZE_CSEC,3) as VALUE
FROM
  v$waitclassmetric  m, v$system_wait_class n
WHERE
  m.wait_class_id=n.wait_class_id AND n.wait_class != 'Idle'
'''

[[metric]]
context = "tablespace"
labels = ["tablespace", "type"]
metricsdesc = { bytes = "Generic counter metric of tablespaces bytes in Oracle.", max_bytes = "Generic counter metric of tablespaces max bytes in Oracle.", free = "Generic counter metric of tablespaces free bytes in Oracle.", used_percent = "Gauge metric showing as a percentage of how much of the tablespace has been used." }
cloudwatchtype = { bytes = "Bytes", max_bytes = "Bytes", free = "Bytes", used_percent = "Percent" }
request = '''
SELECT
    dt.tablespace_name as tablespace,
    dt.contents as type,
    dt.block_size * dtum.used_space as bytes,
    dt.block_size * dtum.tablespace_size as max_bytes,
    dt.block_size * (dtum.tablespace_size - dtum.used_space) as free,
    dtum.used_percent
FROM  dba_tablespace_usage_metrics dtum, dba_tablespaces dt
WHERE dtum.tablespace_name = dt.tablespace_name
ORDER by tablespace
'''

`

// DefaultMetrics is a somewhat hacky way to load the default metrics
func (e *Exporter) DefaultMetrics(logger zerolog.Logger) Metrics {
	var metricsToScrape Metrics
	if e.config.DefaultMetricsFile != "" {
		if _, err := toml.DecodeFile(filepath.Clean(e.config.DefaultMetricsFile), &metricsToScrape); err != nil {
			logger.Error().
				Err(errors.New(err.Error())).
				Msg(fmt.Sprintf("there was an issue while loading specified default metrics file, proceeding to run with default metrics"))
		}
		return metricsToScrape
	}

	if _, err := toml.Decode(defaultMetricsConst, &metricsToScrape); err != nil {
		logger.Error().Err(errors.New(err.Error()))
		panic(errors.New("Error while loading " + defaultMetricsConst))
	}
	return metricsToScrape
}