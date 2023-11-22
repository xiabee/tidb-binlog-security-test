catchError {
    node("build_go1130_binlog") {
        deleteDir()
        def ws = pwd()
        container("golang") {
            dir("/home/jenkins/agent/git/tidb-binlog") {
                if (sh(returnStatus: true, script: '[ -d .git ] && [ -f Makefile ] && git rev-parse --git-dir > /dev/null 2>&1') != 0) {
                    deleteDir()
                }
                try {
                    checkout changelog: false, poll: false, scm: [$class: 'GitSCM', branches: [[name: 'master']], doGenerateSubmoduleConfigurations: false, extensions: [[$class: 'PruneStaleBranch'], [$class: 'CleanBeforeCheckout']], submoduleCfg: [], userRemoteConfigs: [[credentialsId: 'github-sre-bot-ssh', refspec: '+refs/pull/*:refs/remotes/origin/pr/*', url: 'git@github.com:pingcap/tidb-binlog.git']]]
                } catch (error) {
                    retry(2) {
                        echo "checkout failed, retry.."
                        sleep 60
                        if (sh(returnStatus: true, script: '[ -d .git ] && [ -f Makefile ] && git rev-parse --git-dir > /dev/null 2>&1') != 0) {
                            deleteDir()
                        }
                        checkout changelog: false, poll: false, scm: [$class: 'GitSCM', branches: [[name: 'master']], doGenerateSubmoduleConfigurations: false, extensions: [[$class: 'PruneStaleBranch'], [$class: 'CleanBeforeCheckout']], submoduleCfg: [], userRemoteConfigs: [[credentialsId: 'github-sre-bot-ssh', refspec: '+refs/pull/*:refs/remotes/origin/pr/*', url: 'git@github.com:pingcap/tidb-binlog.git']]]
                    }
                }
            }

            dir("go/src/github.com/pingcap/tidb-binlog") {

                stage('Build') {
                    sh """
                        cp -R /home/jenkins/agent/git/tidb-binlog/. ./
                        git checkout -f ${ghprbActualCommit}
                        GOPATH=\$GOPATH:${ws}/go make build
                    """
                }

                stage("Upload") {
                    def filepath = "builds/pingcap/tidb-binlog/pr/${ghprbActualCommit}/centos7/tidb-binlog.tar.gz"
                    def refspath = "refs/pingcap/tidb-binlog/pr/${ghprbPullId}/sha1"

                    timeout(10) {
                        sh """
                        rm -rf .git
                        tar --exclude=tidb-binlog.tar.gz -czvf tidb-binlog.tar.gz bin/*
                        curl -F ${filepath}=@tidb-binlog.tar.gz ${FILE_SERVER_URL}/upload
                        echo "pr/${ghprbActualCommit}" > sha1
                        curl -F ${refspath}=@sha1 ${FILE_SERVER_URL}/upload
                        """
                        // cleanup
                        sh "rm -rf sha1 tidb-binlog.tar.gz"
                    }
                }
            }
        }
    }

    currentBuild.result = "SUCCESS"
}

stage('Summary') {
    def duration = ((System.currentTimeMillis() - currentBuild.startTimeInMillis) / 1000 / 60).setScale(2, BigDecimal.ROUND_HALF_UP)
    def slackmsg = "[#${ghprbPullId}: ${ghprbPullTitle}]" + "\n" +
    "${ghprbPullLink}" + "\n" +
    "${ghprbPullDescription}" + "\n" +
    "Build Result: `${currentBuild.result}`" + "\n" +
    "Elapsed Time: `${duration} mins` " + "\n" +
    "${env.RUN_DISPLAY_URL}"

    if (currentBuild.result != "SUCCESS") {
        slackSend channel: '#jenkins-ci', color: 'danger', teamDomain: 'pingcap', tokenCredentialId: 'slack-pingcap-token', message: "${slackmsg}"
    }
}
