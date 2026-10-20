# Changelog

## [0.6.0](https://github.com/JorisJonkers-dev/estate-dashboard/compare/v0.5.0...v0.6.0) (2026-10-20)


### Features

* show each Project's live pods, its uptime checks and its stack's health ([#52](https://github.com/JorisJonkers-dev/estate-dashboard/issues/52)) ([be3997f](https://github.com/JorisJonkers-dev/estate-dashboard/commit/be3997fb07e54273f6acf1bc360f04aba3804fb7))

## [0.5.0](https://github.com/JorisJonkers-dev/estate-dashboard/compare/v0.4.0...v0.5.0) (2026-10-16)


### Features

* connect with the libpq variables where DATABASE_URL is unset ([#48](https://github.com/JorisJonkers-dev/estate-dashboard/issues/48)) ([e9a10ea](https://github.com/JorisJonkers-dev/estate-dashboard/commit/e9a10eabf5400317b9830204b4ee09d55b940497))
* sign in to Postgres with the rotating login the edge derives ([#50](https://github.com/JorisJonkers-dev/estate-dashboard/issues/50)) ([96831ec](https://github.com/JorisJonkers-dev/estate-dashboard/commit/96831ece8585aa14f748641ae14f8dd3cadb7e0a))

## [0.4.0](https://github.com/JorisJonkers-dev/estate-dashboard/compare/v0.3.2...v0.4.0) (2026-10-15)


### Features

* a list beside another column scrolls on its own ([#46](https://github.com/JorisJonkers-dev/estate-dashboard/issues/46)) ([140e7b2](https://github.com/JorisJonkers-dev/estate-dashboard/commit/140e7b22c608327b027c9cc4bb4a116a1fd50d7a))

## [0.3.2](https://github.com/JorisJonkers-dev/estate-dashboard/compare/v0.3.1...v0.3.2) (2026-10-15)


### Bug Fixes

* drop the collector's lines about a gone container, read a pod from when it started, and name pods by role ([#44](https://github.com/JorisJonkers-dev/estate-dashboard/issues/44)) ([86b4dde](https://github.com/JorisJonkers-dev/estate-dashboard/commit/86b4dde0aa58f4eea65605a11435785f4da4481f))

## [0.3.1](https://github.com/JorisJonkers-dev/estate-dashboard/compare/v0.3.0...v0.3.1) (2026-10-15)


### Bug Fixes

* find containers that just started, date Deployed by release, and read Alertmanager ([#42](https://github.com/JorisJonkers-dev/estate-dashboard/issues/42)) ([33e2e8f](https://github.com/JorisJonkers-dev/estate-dashboard/commit/33e2e8f9f796bc94014938470f6ac5647b38051b))

## [0.3.0](https://github.com/JorisJonkers-dev/estate-dashboard/compare/v0.2.0...v0.3.0) (2026-10-15)


### Features

* the estate screens, logs by release, and the house theme ([#39](https://github.com/JorisJonkers-dev/estate-dashboard/issues/39)) ([2164716](https://github.com/JorisJonkers-dev/estate-dashboard/commit/21647163cdf8ff58ab48b39a56cb7a075a6a9dd1))

## [0.2.0](https://github.com/JorisJonkers-dev/estate-dashboard/compare/v0.1.0...v0.2.0) (2026-10-15)


### Features

* publish the image and the fragment, and run without Alertmanager ([#27](https://github.com/JorisJonkers-dev/estate-dashboard/issues/27)) ([9b3147c](https://github.com/JorisJonkers-dev/estate-dashboard/commit/9b3147c0051197783376a219289b2f7ef1b01b30))


### Bug Fixes

* read the gate's inputs and records from the Release Gate, and hold no ConfigMap access ([#24](https://github.com/JorisJonkers-dev/estate-dashboard/issues/24)) ([110353d](https://github.com/JorisJonkers-dev/estate-dashboard/commit/110353d1e396f4b711a080d7c0708bd4b36405a7))

## 0.1.0 (2026-10-07)


### Features

* read Alertmanager's alerts, record what fires and resolves, and silence an alert there ([#12](https://github.com/JorisJonkers-dev/estate-dashboard/issues/12)) ([653f1f5](https://github.com/JorisJonkers-dev/estate-dashboard/commit/653f1f5ea3c4f5d063b45baf5ab2028b3cf5fda9))
* read each Project's pin, its deploy log and the open issues from the Estate repository ([#19](https://github.com/JorisJonkers-dev/estate-dashboard/issues/19)) ([5d5e31c](https://github.com/JorisJonkers-dev/estate-dashboard/commit/5d5e31cccb3b7028d850493da360229c6a1c4d2a))
* read Flux's sources and units and each gated release from the cluster, getting nothing it cannot name ([#16](https://github.com/JorisJonkers-dev/estate-dashboard/issues/16)) ([85cdc7a](https://github.com/JorisJonkers-dev/estate-dashboard/commit/85cdc7a0805cbbb1b1962f298762d2b1f60fa812))
* scaffold the dashboard: its contract, its project file and its own state ([#6](https://github.com/JorisJonkers-dev/estate-dashboard/issues/6)) ([ec3b99d](https://github.com/JorisJonkers-dev/estate-dashboard/commit/ec3b99d7c430ec3fc0dd912fcbebbf9854ed580d))
* sign admins in through auth, with a session of the dashboard's own ([#8](https://github.com/JorisJonkers-dev/estate-dashboard/issues/8)) ([83f74cd](https://github.com/JorisJonkers-dev/estate-dashboard/commit/83f74cd21403d320b0e988616246119696d0bd0d))


### Bug Fixes

* keep the dashboard's host behind forward-auth, as well as signing admins in itself ([#10](https://github.com/JorisJonkers-dev/estate-dashboard/issues/10)) ([0996813](https://github.com/JorisJonkers-dev/estate-dashboard/commit/09968132c8c5fb4cec151915180f1a13f2c7fbe7))
