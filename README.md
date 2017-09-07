juju-pwhash
===========

A small tool to generate password hashes for juju agents.
The hash can then be inserted into the machine record in MongoDB for a specific unit.
The password will also need to be updated in the unit's agent.conf under `/var/lib/juju/agents/<unit name>/agent.conf`

Usage
-----

```
$ juju-pwhash 
  -p string
          Generate hash for this password (required)
```

Install
=======

`go install github.com/devec0/juju-pwhash`
