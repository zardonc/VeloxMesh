# Dashboard Production Data Implementation Plan

1. Add failing Go and Python tests for real provider health/request log snapshots, then implement Redis contracts and publisher output.
2. Enable Redis AOF in both dashboard and VeloxMesh compose files; verify configuration and restart persistence.
3. Add failing frontend auth tests, replace localStorage role login with BFF login/verification/session/logout.
4. Add comparison helpers/tests for the four methods and matching dataset/setup groups.
5. Add benchmark filters and table usability styles, with component/helper tests.
6. Add Playwright acceptance coverage for role separation, live operational data, comparison filtering, and exports.
7. Run Go, Python, frontend unit, build, Docker persistence, and browser acceptance checks; update the acceptance evidence/report.
