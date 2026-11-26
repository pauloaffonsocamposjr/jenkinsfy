#!/usr/bin/env bash
# Check jenkinsfy output against a real Jenkins running in podman.
#
#   test/jenkins/verify.sh up     build the image and start Jenkins on 127.0.0.1:8080
#   test/jenkins/verify.sh lint   validate every generated Jenkinsfile with Jenkins' linter
#   test/jenkins/verify.sh run    convert sample.yml and build it as a multibranch job
#   test/jenkins/verify.sh all    up + lint + run
#   test/jenkins/verify.sh down   stop and remove the container (keeps the data volume)
#
# Environment: JENKINS_PORT (8080), JENKINS_USER/JENKINS_PASSWORD (admin/admin).
set -euo pipefail

HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
NAME=jenkinsfy-jenkins
IMAGE=localhost/jenkinsfy-jenkins:latest
PORT=${JENKINS_PORT:-8080}
URL="http://127.0.0.1:$PORT"
USER_=${JENKINS_USER:-admin}
PASS=${JENKINS_PASSWORD:-admin}
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

log() { printf '\033[1m==> %s\033[0m\n' "$*"; }

api() { # api METHOD PATH [curl args...]
	local method=$1 path=$2
	shift 2
	if [[ ! -s $WORK/crumb ]]; then
		curl -sf -c "$WORK/cookies" -u "$USER_:$PASS" \
			"$URL/crumbIssuer/api/xml?xpath=concat(//crumbRequestField,%22:%22,//crumb)" >"$WORK/crumb"
	fi
	curl -sS -b "$WORK/cookies" -u "$USER_:$PASS" -H "$(cat "$WORK/crumb")" -X "$method" "$@" "$URL$path"
}

wait_ready() {
	for _ in $(seq 1 100); do
		if [[ $(curl -s -o /dev/null -w '%{http_code}' -u "$USER_:$PASS" "$URL/api/json") == 200 ]]; then
			return 0
		fi
		sleep 3
	done
	echo "Jenkins did not become ready; see: podman logs $NAME" >&2
	return 1
}

cmd_up() {
	log "Building $IMAGE"
	podman build -q -t "$IMAGE" -f "$HERE/Containerfile" "$HERE" >/dev/null
	if podman container exists "$NAME"; then
		podman start "$NAME" >/dev/null
	else
		log "Starting $NAME on $URL"
		podman run -d --name "$NAME" -p "127.0.0.1:$PORT:8080" \
			-v jenkinsfy-jenkins-home:/var/jenkins_home \
			-e JENKINS_ADMIN_USER="$USER_" -e JENKINS_ADMIN_PASSWORD="$PASS" \
			-e JENKINS_DUMMY_SECRETS=REGISTRY_TOKEN,NPM_TOKEN,DEPLOY_KEY,API_TOKEN,OTHER \
			"$IMAGE" >/dev/null
	fi
	wait_ready
	log "Jenkins is up at $URL (user: $USER_)"
}

cmd_down() {
	podman rm -f "$NAME" >/dev/null 2>&1 || true
	log "Removed $NAME (data volume jenkinsfy-jenkins-home kept; 'podman volume rm' it to reset)"
}

cmd_lint() {
	log "Generating Jenkinsfiles from the converter tests"
	mkdir -p "$WORK/dump"
	(cd "$ROOT" && JENKINSFY_DUMP_DIR="$WORK/dump" go test -count=1 ./internal/convert >/dev/null)
	cp "$ROOT"/internal/convert/testdata/*.Jenkinsfile "$WORK/dump/"
	local failed=0 total=0 out
	for f in "$WORK"/dump/*.Jenkinsfile; do
		total=$((total + 1))
		out=$(api POST /pipeline-model-converter/validate --data-urlencode "jenkinsfile@$f")
		if [[ $out == *"successfully validated"* ]]; then
			printf '  ok    %s\n' "$(basename "$f")"
		else
			failed=$((failed + 1))
			printf '  FAIL  %s\n%s\n' "$(basename "$f")" "$out" | sed 's/^/        /'
		fi
	done
	log "Lint: $((total - failed))/$total passed"
	[[ $failed == 0 ]]
}

cmd_run() {
	local workflow=${1:-$HERE/sample.yml} job=jenkinsfy-sample
	log "Converting $(basename "$workflow")"
	(cd "$ROOT" && go run . -label ubuntu-latest=built-in -o "$WORK/Jenkinsfile" "$workflow")

	log "Publishing it to a git repository inside the container"
	podman cp "$WORK/Jenkinsfile" "$NAME:/tmp/Jenkinsfile"
	podman exec "$NAME" bash -euc '
		rm -rf /var/jenkins_home/jenkinsfy-sample.git /tmp/sample
		git init -q --bare -b main /var/jenkins_home/jenkinsfy-sample.git
		git init -q -b main /tmp/sample && cd /tmp/sample
		cp /tmp/Jenkinsfile .
		git add Jenkinsfile
		git -c user.name=jenkinsfy -c user.email=jenkinsfy@localhost commit -qm "jenkinsfy sample"
		git push -q /var/jenkins_home/jenkinsfy-sample.git main'

	log "Creating multibranch job '$job'"
	api POST "/job/$job/doDelete" -o /dev/null || true
	api POST "/createItem?name=$job" -H 'Content-Type: application/xml' --data-binary @- -o /dev/null <<'XML'
<org.jenkinsci.plugins.workflow.multibranch.WorkflowMultiBranchProject>
  <sources class="jenkins.branch.MultiBranchProject$BranchSourceList">
    <data>
      <jenkins.branch.BranchSource>
        <source class="jenkins.plugins.git.GitSCMSource">
          <id>jenkinsfy-sample</id>
          <remote>file:///var/jenkins_home/jenkinsfy-sample.git</remote>
          <traits><jenkins.plugins.git.traits.BranchDiscoveryTrait/></traits>
        </source>
      </jenkins.branch.BranchSource>
    </data>
  </sources>
  <factory class="org.jenkinsci.plugins.workflow.multibranch.WorkflowBranchProjectFactory">
    <scriptPath>Jenkinsfile</scriptPath>
  </factory>
</org.jenkinsci.plugins.workflow.multibranch.WorkflowMultiBranchProject>
XML
	api POST "/job/$job/build?delay=0" -o /dev/null

	log "Waiting for build main #1"
	local result=""
	for _ in $(seq 1 120); do
		# Wait for building=false: catchError() can set an interim result.
		result=$(api GET "/job/$job/job/main/1/api/json?tree=building,result" 2>/dev/null |
			grep '"building":false' | sed -n 's/.*"result":"\([A-Z_]*\)".*/\1/p') || true
		[[ -n $result ]] && break
		sleep 2
	done
	api GET "/job/$job/job/main/1/consoleText" >"$WORK/console.txt" || true
	cp "$WORK/console.txt" "$HERE/last-console.txt"
	log "Result: ${result:-TIMEOUT} (console: test/jenkins/last-console.txt, UI: $URL/job/$job/job/main/1/)"
	[[ $result == SUCCESS ]]
}

case ${1:-all} in
up) cmd_up ;;
down) cmd_down ;;
lint) cmd_lint ;;
run) shift; cmd_run "$@" ;;
all) cmd_up && cmd_lint && cmd_run ;;
*) sed -n '2,10p' "$0"; exit 1 ;;
esac
