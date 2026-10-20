# compare real paths on macos

temp dirs live under /var, which is a link to /private/var. EvalSymlinks returns the /private form, so comparing it to an unresolved path fails. resolve both sides before comparing.
