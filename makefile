default: clean build

-include ".env"

# URL path prefix nginx serves finished builds under (departer.sh-tmpl
# echoes it as the download link). Default here so a regenerate can't lose it.
DEPARTER_STATIC ?= /departer/builds

# Origin the builds are publicly reachable at — prepended to the download link
# the build log ends with, so the link is absolute (and clickable) in the CMS.
# The staging orchestrator overrides this per ticket; production uses this.
DEPARTER_PUBLIC_BASE ?= https://paver.energyaccessexplorer.org

DEPARTER_CMD = departer \
	-role admin \
	-role leader \
	-role manager \
	-role director \
	-role root \
	-script ${DEPARTER_SCRIPT} \
	-tmpdir ${DEPARTER_TMPDIR} \
	-pubkey ${DEPARTER_PUBKEY}

COMMIT_SHA != git rev-parse --short HEAD

build:
	go get
	go fmt
	go build -ldflags "-s -X main.COMMIT_SHA=${COMMIT_SHA}"

.export DEPARTER_CMD
.export DEPARTER_SOCKET
.export DEPARTER_WORKSPACE
.export DEPARTER_TMPDIR
.export DEPARTER_USER
.export DEPARTER_STATIC
.export DEPARTER_PUBLIC_BASE
.export OFFROAD_WORKSPACE
	@envsubst <departer.service-tmpl >departer.service
	@envsubst <departer.sh-tmpl >departer.sh

	@chmod +x departer.sh

clean:
	-rm -f departer departer.service departer.sh

install:
	git pull
	bmake build
	sudo install -o root -m 755 \
		departer \
		/usr/local/bin/

	sudo install -o root -m 755 \
		departer.sh \
		/usr/local/bin/

	sudo install -o root -g root -m 644 \
		departer.service \
		/etc/systemd/system/

deploy:
	rsync --recursive --verbose \
		--exclude="makefile" \
		--exclude="departer" \
		--exclude=".git*" \
		--exclude="go.mod" \
		./ eae-paver:~/departer

	ssh ${DEPARTER_SERVER} "sudo systemctl stop departer.service"
	ssh ${DEPARTER_SERVER} "cd ${DEPARTER_WORKSPACE}; bmake install;"
	ssh ${DEPARTER_SERVER} "sudo systemctl daemon-reload"
	ssh ${DEPARTER_SERVER} "sudo systemctl start departer.service"
