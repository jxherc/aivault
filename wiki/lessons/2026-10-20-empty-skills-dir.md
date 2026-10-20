# a "" folder is the current folder

aider has no skills folder, so its Skills() returned "". filepath.Join("", name) is a relative path, and the link landed in the package dir during tests. skip empty dirs, and test that nothing appears in the cwd.
