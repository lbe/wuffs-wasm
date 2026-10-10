# M / L benchmark fixtures (not in git)

Medium (`m/`) and large (`l/`) decode benchmark inputs are downloaded from
[google/wuffs `test/data`](https://github.com/google/wuffs/tree/main/test/data)
at pinned commit `1f513d802343ec2f8a5c6e987cd071e655d36936` recorded in
`manifest.tsv`. The fetch URL template is
`https://raw.githubusercontent.com/google/wuffs/<WUFFS_COMMIT>/test/data/<basename>`.

```bash
make bench-fixtures   # from repo root; requires network (curl)
```

Small (S-tier) fixtures used by BENCH-01 remain under committed `testdata/`.

## License

Wuffs test data is distributed under the MIT license and Apache License 2.0.
See <https://github.com/google/wuffs/blob/main/LICENSE>.
