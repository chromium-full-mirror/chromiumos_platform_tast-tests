rollback_smoke_metrics_file simulates a freshly created
`/mnt/stateful_partition/unencrypted/preserve/enterprise-rollback-metrics-data`

It contains an encrypted `EnterpriseRollbackMetricsData` proto with the following
information:

- `RollbackMetadata` with origin chromeOS version 15621 and target chromeOS
version 15474.

- ROLLBACK_POLICY_ACTIVATED event.

The decrypted proto file looks as follows:

```
1 {
  1 {
    1: 15621
  }
  2 {
    1: 15474
  }
}
2 {
  1: 1
}
```
