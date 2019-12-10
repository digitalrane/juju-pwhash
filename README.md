# juju-pwhash

A small tool to generate password hashes for juju agents.
The hash can then be inserted into the machine record in MongoDB for a specific unit.
The password will also need to be updated in the unit's agent.conf under `/var/lib/juju/agents/<unit name>/agent.conf`

## Usage

```
$ juju-pwhash
  -p string
      Generate hash for this password (required).
  -s string
      If provided, generate a user hash instead of machine hash using this salt for this password (optional).
```

# Install

`go get github.com/devec0/juju-pwhash`


# Saving a Juju model

Here's what happened.  We upgraded a controller from 2.4.7 to 2.5.8 (auto selected version).  The controller upgrade went wrong, and an ancient model overwrote a bunch of machine/unit agents when it got
(somehow) started again.  The machine agents 'suicided' and removed themselves, along with the units etc.  However, the machines were running and the cloud was running OK.  We then tried to restore a known
good backup of the Juju database, which requires a non-ha model.  The commands to remove controller machines was introduced in 2.5, but the upgrade had got far enough to allow this to happen.  Restoring the
database put back the machines in the Juju model that were missing from the older model, and also the HA controllers, but did not repair the damage done by the agents to themselves in attempting to make them
look like the outdated controller.  Recovery (or redeploy) was therefore necessary.

Open sessions on:

- The Juju database (don't use juju ssh, you'll probably be stopping the model)
- Each controller machine
- a Juju client.


Tools needed:

* [[https://github.com/devec0/juju-pwhash]]
* Mongodb client
* Juju client
* pwgen (or similar)

Data to collect:

Get the model uuid from the Juju db:

```
juju:PRIMARY> db.controllers.find().pretty()
{
        "_id" : "e",
        "cloud" : "foundation-maas",
        "model-uuid" : "c7006796-e378-47c9-8209-cf74d0936200",
        "machineids" : [
                "0",
                "1",
                "2"
        ],
```
In this case, we want: `"model-uuid" : "c7006796-e378-47c9-8209-cf74d0936200",`


Hopefully, you'll see auditing enabled for later analysis:

```
juju:PRIMARY> db.settings.find({_id:"c7006796-e378-47c9-8209-cf74d0936200:e"}).pretty()
{
        "_id" : "c7006796-e378-47c9-8209-cf74d0936200:e",
        "model-uuid" : "c7006796-e378-47c9-8209-cf74d0936200",
        "settings" : {
                "auditing-enabled" : true,
<snippy snip>
```

Check sanity for the model:

```
juju:PRIMARY> db.units.count()
947
juju:PRIMARY> db.machines.count()
121

```

## Recover things

### Pre-req

First, recover the Mongo database so there's a good cluster of 3 units.  Juju will not operate correctly if it expects Mongo to be clustered, but `rs.status()` doesn't show good output.

### Juju controllers

Check the status of the agents on a controller machine, on the agent.conf.  You may need to edit the machine agent.conf (this was already done here):

```
ubuntu@juju-1:/var/lib/juju/agents$ sudo grep 'upgraded' */agent.conf
machine-0/agent.conf:upgradedToVersion: 2.4.7
unit-filebeat-controller-1/agent.conf:upgradedToVersion: 2.5.8
unit-juju-controller-0/agent.conf:upgradedToVersion: 2.5.8
unit-landscape-client-controller-1/agent.conf:upgradedToVersion: 2.5.8
unit-nrpe-controller-0/agent.conf:upgradedToVersion: 2.5.8
unit-ntp-1/agent.conf:upgradedToVersion: 2.5.8
unit-telegraf-controller-0/agent.conf:upgradedToVersion: 2.5.8
```

The juju-db configuration had been modified in this instance by the 2.5 upgrade, and left us unable to start the rolled back Juju agent because of https://bugs.launchpad.net/juju/+bug/1820327 - there's notes
in the bug report to clear this - we had to remove all mentions of `unlimited` from `/etc/systemd/system/juju-db.service`.

Restarting the machine agent, with the agent.conf set correctly, should allow the agent to start correctly.
We then restarted the units, one at a time, and the restart corrected agent.conf - we restarted once more "just to be sure" afterwards.

This was repeated across all three controller machines, to bring us back to an HA cluster.

If there's no Juju unit agents running on the controllers (typically if you're not monitoring them etc), then it's just a matter of restarting the jujud-machine-X.service after editing agent.conf to the
correct version (in our case, 2.4.7).

### Actual Juju model machine agents

The `juju status` output in this case, once the controller was alive and well, was showing some machines working fine, some units 'lost', and their machines 'down'.  These were the ones that we needed to
recover.


Taking the example of a machine that had no agent at all, neither machine nor unit agents, we needed to download the tools and make the machine agent service from scratch:

Set up, for example if this is machine 129. Note, if the machine is a container, it'll have the format 1-lxd-4 rather than 1/lxd/4 in (most) cases.
```
export machine=129
```
Or, if it's machine 69/lxd/3:
```
export machine=69-lxd-3
```

We need to collect some info from the Juju db, and change an item too.

To get the value for `nonce` (this one is machine 129):

(If this is an LXD, it'll be something like `db.machines.find({"machineid": "69/lxd/3"}).pretty()`)

```
juju:PRIMARY> db.machines.find({"machineid": "129"}, { "machineid" :1, "nonce" :1}).pretty()
{
        "_id" : "2f55d390-44a1-4dee-859b-2350a9e71ac3:129",
        "machineid" : "129",
        "nonce" : "machine-0:9f6f01a5-35a1-40c8-8b21-75ffb0c376ce"
}
```
Save that _id, it'll come in handy in a minute.

To create a password for `apipassword`:

```
$ pwgen 24 1
gimmie24randomcharsplease
$ juju-pwhash -p gimmie24randomcharsplease
thisisthehashfromabove
```

Now we need to update that password hash in the db:
```

juju:PRIMARY> db.machines.update({_id: "2f55d390-44a1-4dee-859b-2350a9e71ac3:129"}, {$set: {"passwordhash" : "thisisthehashfromabove"}})
```

On your target machine, using the data from above:
```
export nonce=machine-0:9f6f01a5-35a1-40c8-8b21-75ffb0c376ce
export apipassword=gimmie24randomcharsplease

```
Grab the tools:

```
mkdir -p /var/lib/juju/tools/2.4.7-xenial-amd64
curl -k -o /var/lib/juju/tools/2.4.7-xenial-amd64/tools.tar.gz https://1.2.3.4:17070/model/2f55d390-44a1-4dee-859b-2350a9e71ac3/tools/2.4.7-xenial-amd64
cd /var/lib/juju/tools/2.4.7-xenial-amd64
tar -zxf tools.tar.gz
cat << EOF >/var/lib/juju/tools/2.4.7-xenial-amd64/downloaded-tools.txt
{"version":"2.4.7-xenial-amd64","url":"https://1.2.3.4:17070/model/2f55d390-44a1-4dee-859b-2350a9e71ac3/tools/2.4.7-xenial-amd64","sha256":"1812f497766d67c64a93dfa7f4dab7e16701cdea4b83f583fbc88ba2ea5493f5","size":26202480}
EOF
```

Make the link for the tools:
```
cd /var/lib/juju/tools
ln -s /var/lib/juju/tools/2.4.7-xenial-amd64 machine-${machine}
```

Fix the service files.  It's likely that the systemd things left behind are still there, which will stop the machine agent deployer from redeploying the unit agents.  Need to remove these:

```
rm /etc/systemd/system/jujud-unit-*
rm -rf /lib/systemd/system/jujud-unit-*

```

The agent config needs adding next. There's a few items to edit here: the `tag` needs to match the correct machine agent name, the `nonce` needs to match what is in the database, and the `AGENT_SERVICE_NAME` needs fixing.  We need to create a
new password and fix the database to match it.


Example agent.conf as a donor:
```
mkdir -p /var/lib/juju/agents/machine-${machine}
cat << EOF >/var/lib/juju/agents/machine-${machine}/agent.conf
# format 2.0
tag: machine-${machine}
datadir: /var/lib/juju
logdir: /var/log/juju
metricsspooldir: /var/lib/juju/metricspool
nonce: ${nonce}
jobs:
- JobHostUnits
upgradedToVersion: 2.4.7
cacert: |
  -----BEGIN CERTIFICATE-----
  youllhavetogofindthisfromanexistingagent.conf
  -----END CERTIFICATE-----
controller: controller-51e5a990-a9a2-4f1a-82a0-f6f5c0d64f03
model: model-2f55d390-44a1-4dee-859b-2350a9e71ac3
apiaddresses:
- 1.3.6.8:17070
- 1.3.11.7:17070
- 1.2.3.4:17070
apipassword: ${apipassword}
loggingconfig: <root>=DEBUG;unit=DEBUG
values:
  AGENT_SERVICE_NAME: jujud-machine-${machine}
  CONTAINER_TYPE: ""
  PROVIDER_TYPE: maas
mongoversion: "0.0"


EOF
```


Next, we make the systemd service entries:

```
mkdir -p /lib/systemd/system/jujud-machine-$machine

```

Example for `/lib/systemd/system/jujud-machine-129/exec-start.sh`:

```
cat << EOF | sed s/129/$machine/g >/lib/systemd/system/jujud-machine-${machine}/exec-start.sh
#!/usr/bin/env bash

# Set up logging.
touch '/var/log/juju/machine-129.log'
chown syslog:syslog '/var/log/juju/machine-129.log'
chmod 0600 '/var/log/juju/machine-129.log'
exec >> '/var/log/juju/machine-129.log'
exec 2>&1

# Run the script.
'/var/lib/juju/tools/machine-129/jujud' machine --data-dir '/var/lib/juju' --machine-id 129 --debug

EOF

```

Note: here we need to ensure that for containers, `--machine-id 129` matches `--machine-id 1/lxd/2` rather than `1-lxd-2`.

Make the service file itself:

```
cat << EOF | sed s/129/$machine/g >/lib/systemd/system/jujud-machine-${machine}/jujud-machine-${machine}.service
[Unit]
Description=juju agent for machine-129
After=syslog.target
After=network.target
After=systemd-user-sessions.service

[Service]
LimitNOFILE=20000
ExecStart=/lib/systemd/system/jujud-machine-129/exec-start.sh
Restart=on-failure
TimeoutSec=300

[Install]
WantedBy=multi-user.target

EOF

```

Fix permissions and make the links:

```
chmod u+x /lib/systemd/system/jujud-machine-${machine}/exec-start.sh
cd /etc/systemd/system
ln -s /lib/systemd/system/jujud-machine-${machine}/jujud-machine-${machine}.service
systemctl daemon-reload

```

Brave time: start the agent.
```
systemctl start jujud-machine-$machine
systemctl status jujud-machine-$machine
```

### fixing the unit agents if the auto redeploy doesn't work

Once the machine agents are alive, we can start to look at the units that aren't healthy.

The process is much the same as for the machine agent, except we don't need to find/fix the `nonce` in agent.conf.

Let's start with ceph-osd/27, which lives on machine 129 (see above).  It has no agent at all right now.

Get the tools ready:
```
cd /var/lib/juju/tools
ln -s /var/lib/juju/tools/2.4.7-xenial-amd64 unit-ceph-osd-27
```

Make a new password and hash for the agent.conf (see above section).

The machine agent (should) make the systemd configs, but if not, see above.

Example agent config from a good host:

```
sudo cat unit-ceph-osd-17/agent.conf
# format 2.0
tag: unit-ceph-osd-17
datadir: /var/lib/juju
logdir: /var/log/juju
metricsspooldir: /var/lib/juju/metricspool
nonce: unused
upgradedToVersion: 2.4.7
cacert: |
  -----BEGIN CERTIFICATE-----
  redactedofcourse
  -----END CERTIFICATE-----
controller: controller-51e5a990-a9a2-4f1a-82a0-f6f5c0d64f03
model: model-2f55d390-44a1-4dee-859b-2350a9e71ac3
apiaddresses:
- 1.2.3.4:17070
apipassword: somepassword
loggingconfig: <root>=DEBUG;unit=DEBUG
values:
  CONTAINER_TYPE: ""
  NAMESPACE: ""
mongoversion: "0.0"
```

Update the `apipassword` and `tag`.

Fix the hash in the db:
```
db.units.update({"_id" : "2f55d390-44a1-4dee-859b-2350a9e71ac3:ceph-osd/27"}, { $set: {"passwordhash" : "thenewhash"}})
```

Note the format for `_id` is "<model-uuid>:<unit name>"

