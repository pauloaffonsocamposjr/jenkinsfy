// Local test instance setup: admin user, executors and dummy credentials.
import jenkins.model.Jenkins
import hudson.security.HudsonPrivateSecurityRealm
import hudson.security.FullControlOnceLoggedInAuthorizationStrategy
import com.cloudbees.plugins.credentials.CredentialsScope
import com.cloudbees.plugins.credentials.SystemCredentialsProvider
import com.cloudbees.plugins.credentials.domains.Domain
import org.jenkinsci.plugins.plaincredentials.impl.StringCredentialsImpl
import hudson.util.Secret

def j = Jenkins.get()
def user = System.getenv('JENKINS_ADMIN_USER') ?: 'admin'
def pass = System.getenv('JENKINS_ADMIN_PASSWORD') ?: 'admin'

def realm = new HudsonPrivateSecurityRealm(false)
if (realm.getUser(user) == null) {
    realm.createAccount(user, pass)
}
j.setSecurityRealm(realm)
def auth = new FullControlOnceLoggedInAuthorizationStrategy()
auth.setAllowAnonymousRead(false)
j.setAuthorizationStrategy(auth)
j.setNumExecutors(4)

// Secret-text credentials referenced by converted workflows, filled with dummy values.
def ids = (System.getenv('JENKINS_DUMMY_SECRETS') ?: '').split(',')*.trim().findAll { it }
def store = SystemCredentialsProvider.getInstance().getStore()
ids.each { id ->
    def existing = store.getCredentials(Domain.global()).find { it.id == id }
    if (!existing) {
        store.addCredentials(Domain.global(),
            new StringCredentialsImpl(CredentialsScope.GLOBAL, id, 'jenkinsfy dummy secret', Secret.fromString("dummy-${id}")))
    }
}
j.save()
