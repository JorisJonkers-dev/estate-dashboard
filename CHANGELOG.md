# Changelog

## 0.1.0 (2026-10-07)


### Features

* read Alertmanager's alerts, record what fires and resolves, and silence an alert there ([#12](https://github.com/JorisJonkers-dev/estate-dashboard/issues/12)) ([653f1f5](https://github.com/JorisJonkers-dev/estate-dashboard/commit/653f1f5ea3c4f5d063b45baf5ab2028b3cf5fda9))
* read each Project's pin, its deploy log and the open issues from the Estate repository ([#19](https://github.com/JorisJonkers-dev/estate-dashboard/issues/19)) ([5d5e31c](https://github.com/JorisJonkers-dev/estate-dashboard/commit/5d5e31cccb3b7028d850493da360229c6a1c4d2a))
* read Flux's sources and units and each gated release from the cluster, getting nothing it cannot name ([#16](https://github.com/JorisJonkers-dev/estate-dashboard/issues/16)) ([85cdc7a](https://github.com/JorisJonkers-dev/estate-dashboard/commit/85cdc7a0805cbbb1b1962f298762d2b1f60fa812))
* scaffold the dashboard: its contract, its project file and its own state ([#6](https://github.com/JorisJonkers-dev/estate-dashboard/issues/6)) ([ec3b99d](https://github.com/JorisJonkers-dev/estate-dashboard/commit/ec3b99d7c430ec3fc0dd912fcbebbf9854ed580d))
* sign admins in through auth, with a session of the dashboard's own ([#8](https://github.com/JorisJonkers-dev/estate-dashboard/issues/8)) ([83f74cd](https://github.com/JorisJonkers-dev/estate-dashboard/commit/83f74cd21403d320b0e988616246119696d0bd0d))


### Bug Fixes

* keep the dashboard's host behind forward-auth, as well as signing admins in itself ([#10](https://github.com/JorisJonkers-dev/estate-dashboard/issues/10)) ([0996813](https://github.com/JorisJonkers-dev/estate-dashboard/commit/09968132c8c5fb4cec151915180f1a13f2c7fbe7))
