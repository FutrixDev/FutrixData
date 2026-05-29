<template>
  <section class="view active" id="view-form">
    <div class="list-toolbar">
      <div>
        <h2>{{ formTitle }}</h2>
        <p class="meta">{{ tApp('datasource.form.subtitle') }}</p>
      </div>
      <div class="console-actions">
        <button class="btn secondary" type="button" @click="cancel">{{ tApp('common.cancel') }}</button>
        <button class="btn" type="button" @click="save">{{ tApp('common.save') }}</button>
      </div>
    </div>

    <div class="panel">
      <div v-if="errors.length" class="form-errors show">
        <div v-for="(err, idx) in errors" :key="idx">{{ err }}</div>
      </div>

      <div class="form-grid">
        <div>
          <label for="ds-name">{{ tApp('common.name') }}</label>
          <input id="ds-name" v-model="form.name" placeholder="analytics-prod" :class="fieldClass('name')" v-bind="textInputAttrs" />
        </div>
        <div>
          <label for="ds-type">{{ tApp('common.type') }}</label>
          <DatasourceTypeSelect
            id="ds-type"
            v-model="form.type"
            :options="dataSourceTypeOptions"
            :trigger-class="fieldClass('type')"
          />
        </div>
        <div v-if="isDynamo" class="field">
          <label for="ddb-region">{{ tApp('datasource.form.region') }}</label>
          <input
            id="ddb-region"
            v-model="form.dynamoRegion"
            placeholder="us-east-1"
            :class="fieldClass('dynamoRegion')"
            @input="markDynamoRegionAsManual"
            v-bind="textInputAttrs"
          />
        </div>
        <div v-if="isDynamo" class="field">
          <label for="ddb-auth-mode">{{ tApp('datasource.form.dynamo.authMode') }}</label>
          <select id="ddb-auth-mode" v-model="form.dynamoAuthMode">
            <option value="sso">{{ tApp('datasource.form.dynamo.authMode.sso') }}</option>
            <option value="profile">{{ tApp('datasource.form.dynamo.authMode.profile') }}</option>
          </select>
        </div>
        <div v-if="isDynamo && isDynamoSSO" class="field span-2">
          <label for="ddb-sso-profile">{{ tApp('datasource.form.dynamo.ssoProfile') }}</label>
          <div class="console-actions">
            <select id="ddb-sso-profile" v-model="form.dynamoProfile" :class="fieldClass('dynamoProfile')">
              <option value="" disabled>{{ tApp('datasource.form.dynamo.ssoSelectProfile') }}</option>
              <option v-for="profile in dynamoSSOProfiles" :key="profile.name" :value="profile.name">
                {{
                  profile.region
                    ? tApp('datasource.form.dynamo.ssoProfileOption', { profile: profile.name, region: profile.region })
                    : profile.name
                }}
              </option>
            </select>
            <button
              id="ddb-sso-load-profiles"
              class="btn secondary"
              type="button"
              :disabled="dynamoSSOProfilesLoading"
              @click="loadDynamoSSOProfiles"
            >
              {{
                dynamoSSOProfilesLoading
                  ? tApp('datasource.form.dynamo.ssoLoadProfilesLoading')
                  : tApp('datasource.form.dynamo.ssoLoadProfiles')
              }}
            </button>
          </div>
          <p v-if="!dynamoSSOProfilesLoading && !dynamoSSOProfiles.length" class="meta">
            {{ tApp('datasource.form.dynamo.ssoNoProfiles') }}
          </p>
        </div>
        <div v-if="isDynamo && isDynamoSSO" class="field span-2">
          <label for="ddb-sso-config-path">{{ tApp('datasource.form.dynamo.ssoConfigPathOptional') }}</label>
          <div class="dynamo-config-path-row">
            <input
              id="ddb-sso-config-path"
              v-model="form.dynamoSSOConfigPath"
              :placeholder="tApp('datasource.form.dynamo.ssoConfigPathPlaceholder')"
              v-bind="textInputAttrs"
            />
            <button
              id="ddb-sso-config-apply"
              class="btn success"
              type="button"
              :disabled="dynamoSSOProfilesLoading || dynamoSSOConfigApplyLoading"
              @click="applyDynamoSSOConfigPath"
            >
              {{
                dynamoSSOConfigApplyLoading
                  ? tApp('datasource.form.dynamo.ssoConfigApplyLoading')
                  : tApp('datasource.form.dynamo.ssoConfigApply')
              }}
            </button>
          </div>
          <p class="meta">{{ tApp('datasource.form.dynamo.ssoConfigPathHint') }}</p>
        </div>
        <div v-if="isDynamo && isDynamoSSO && dynamoSSOHasConfigEndpoint" class="field span-2">
          <label>{{ tApp('datasource.form.dynamo.ssoEndpointFromConfig') }}</label>
          <p class="meta">{{ dynamoSSOConfigEndpoint }}</p>
        </div>
        <div v-if="isDynamo && !isDynamoSSO" class="field">
          <label for="ddb-profile">{{ tApp('datasource.form.profileOptional') }}</label>
          <input id="ddb-profile" v-model="form.dynamoProfile" placeholder="default" v-bind="textInputAttrs" />
        </div>
        <div v-if="isDynamo && (!isDynamoSSO || !dynamoSSOHasConfigEndpoint)" class="field span-2">
          <label for="ddb-endpoint">{{ tApp('datasource.form.endpointOptional') }}</label>
          <input id="ddb-endpoint" v-model="form.dynamoEndpoint" placeholder="http://127.0.0.1:8000" v-bind="textInputAttrs" />
          <p class="meta">{{ tApp('datasource.form.endpointHint') }}</p>
        </div>
        <div v-if="isDynamo && isDynamoSSO" class="field span-2">
          <label for="ddb-sso-oauth">{{ tApp('datasource.form.dynamo.ssoOauth') }}</label>
          <div class="console-actions">
            <button
              id="ddb-sso-oauth"
              :class="['btn', dynamoSSOConnected ? 'success' : 'secondary']"
              type="button"
              :disabled="dynamoSSOOAuthLoading"
              @click="dynamoSSOOAuthAuthorize"
            >
              {{
                dynamoSSOOAuthLoading
                  ? tApp('datasource.form.dynamo.ssoOauthLoading')
                  : dynamoSSOConnected
                    ? tApp('status.connected')
                    : tApp('datasource.form.dynamo.ssoOauth')
              }}
            </button>
          </div>
          <p class="meta">{{ tApp('datasource.form.dynamo.ssoOauthHint') }}</p>
          <p v-if="dynamoSSOConnected" class="meta">
            {{ tApp('datasource.form.dynamo.ssoAuthorizedContext', { accountId: form.dynamoSSOAccountId, roleName: form.dynamoSSORoleName }) }}
          </p>
        </div>
        <div v-if="isDynamo && !isDynamoSSO" class="field span-2">
          <label for="ddb-static-creds" class="checkbox-inline-label">
            <input id="ddb-static-creds" type="checkbox" v-model="form.dynamoUseStaticCreds" />
            {{ tApp('datasource.form.useStaticCredentialsOptional') }}
          </label>
          <p class="meta">{{ tApp('datasource.form.staticCredentialsRecommended') }}</p>
        </div>
        <div v-if="isDynamo && !isDynamoSSO" class="field span-2">
          <label for="ddb-credentials-file">{{ tApp('datasource.form.uploadCredentialsOptional') }}</label>
          <input
            id="ddb-credentials-file"
            type="file"
            accept=".csv,.txt,.ini"
            @change="handleDynamoCredentialsFile"
          />
          <p class="meta">{{ tApp('datasource.form.credentialsFileHint') }}</p>
        </div>
        <div v-if="isDynamo && (form.dynamoUseStaticCreds || isDynamoSSO)" class="field">
          <label for="ddb-access-key-id">{{ tApp('datasource.form.accessKeyId') }}</label>
          <input id="ddb-access-key-id" v-model="form.dynamoAccessKeyId" :readonly="isDynamoSSO" autocomplete="off" v-bind="textInputAttrs" />
        </div>
        <div v-if="isDynamo && (form.dynamoUseStaticCreds || isDynamoSSO)" class="field">
          <label for="ddb-secret-access-key">{{ tApp('datasource.form.secretAccessKey') }}</label>
          <template v-if="isDynamoSSO">
            <div class="dynamo-sensitive-inline">
              <input
                id="ddb-secret-access-key"
                :value="maskedDynamoSecretAccessKey"
                readonly
                autocomplete="off"
                v-bind="textInputAttrs"
              />
              <button
                id="ddb-copy-secret-access-key"
                class="btn ghost mini"
                type="button"
                :disabled="!form.dynamoSecretAccessKey"
                @click="copyDynamoSecretAccessKey"
              >
                {{ tApp('common.copy') }}
              </button>
            </div>
          </template>
          <div v-else class="password-toggle-wrapper">
            <input
              id="ddb-secret-access-key"
              v-model="form.dynamoSecretAccessKey"
              :type="showDynamoSecret ? 'text' : 'password'"
              autocomplete="off"
              v-bind="textInputAttrs"
            />
            <button type="button" class="password-toggle-btn" @click="showDynamoSecret = !showDynamoSecret" tabindex="-1">
              <EyeOff v-if="showDynamoSecret" :size="16" />
              <Eye v-else :size="16" />
            </button>
          </div>
        </div>
        <div v-if="isDynamo && (form.dynamoUseStaticCreds || isDynamoSSO)" class="field span-2">
          <label for="ddb-session-token">{{ tApp('datasource.form.sessionTokenOptional') }}</label>
          <template v-if="isDynamoSSO">
            <div class="dynamo-sensitive-token">
              <textarea
                id="ddb-session-token"
                :value="maskedDynamoSessionToken"
                readonly
                rows="2"
                autocomplete="off"
                v-bind="textInputAttrs"
              ></textarea>
              <button
                id="ddb-copy-session-token"
                class="btn ghost mini dynamo-sensitive-token-copy"
                type="button"
                :disabled="!form.dynamoSessionToken"
                @click="copyDynamoSessionToken"
              >
                {{ tApp('common.copy') }}
              </button>
            </div>
          </template>
          <textarea
            v-else
            id="ddb-session-token"
            v-model="form.dynamoSessionToken"
            rows="2"
            autocomplete="off"
            v-bind="textInputAttrs"
          ></textarea>
        </div>
        <div v-if="isD1" class="field span-2">
          <label for="d1-oauth-login">{{ tApp('datasource.form.d1.oauth') }}</label>
          <div class="console-actions">
            <button id="d1-oauth-login" :class="['btn', d1OAuthVerified ? 'success' : 'secondary']" type="button" :disabled="d1OAuthLoading" @click="d1OAuthLogin">
              {{
                d1OAuthLoading
                  ? tApp('datasource.form.d1.oauthLoading')
                  : d1OAuthVerified
                    ? tApp('status.connected')
                    : d1OAuthAuthenticated
                      ? tApp('datasource.form.d1.oauthRelogin')
                      : tApp('datasource.form.d1.oauthLogin')
              }}
            </button>
          </div>
          <p
            v-if="d1WranglerMissing"
            class="meta d1-oauth-warning"
            data-testid="d1-wrangler-missing-warning"
          >
            {{ tApp('datasource.form.d1.wranglerInstallHint') }}
          </p>
        </div>
        <div v-if="isD1 && d1ConnectionEstablished" class="field span-2">
          <label for="d1-account-select">{{ tApp('datasource.form.d1.account') }}</label>
          <select
            id="d1-account-select"
            v-model="form.d1AccountId"
            :class="fieldClass('d1Oauth')"
            @change="handleD1AccountSelection"
          >
            <option value="" disabled>{{ tApp('datasource.form.d1.selectAccount') }}</option>
            <option v-for="account in d1Accounts" :key="account.id" :value="account.id">
              {{ `${account.name} (${account.id})` }}
            </option>
          </select>
        </div>
        <div v-if="showD1DatabaseSelector" class="field span-2">
          <label for="d1-database-select">{{ tApp('datasource.form.d1.database') }}</label>
          <select
            id="d1-database-select"
            v-model="form.d1DatabaseId"
            :class="fieldClass('d1DatabaseId')"
            @change="handleD1DatabaseSelection"
          >
            <option value="" disabled>{{ tApp('datasource.form.d1.selectDatabase') }}</option>
            <option v-if="d1CanCreateDatabase" :value="d1DatabaseCreateOptionValue">{{ tApp('datasource.form.d1.createDatabaseOption') }}</option>
            <option v-for="item in d1Databases" :key="item.id" :value="item.id">
              {{ tApp('datasource.form.d1.databaseOptionLabel', { name: item.name, id: item.id }) }}
            </option>
          </select>
          <p v-if="d1DatabasesLoading" class="meta">{{ tApp('datasource.form.d1.loadingDatabases') }}</p>
          <p v-else-if="!d1Databases.length" class="meta">{{ tApp('datasource.form.d1.noDatabases') }}</p>
        </div>
        <div v-if="isD1" class="field span-2">
          <label for="d1-support-dev" class="checkbox-inline-label">
            <input id="d1-support-dev" type="checkbox" v-model="form.d1SupportDev" @change="markD1SupportDevTouched" />
            {{ tApp('datasource.form.d1.supportDev') }}
          </label>
          <p class="meta">{{ tApp('datasource.form.d1.supportDevHint') }}</p>
        </div>
        <div v-if="isD1 && form.d1SupportDev" class="field span-2">
          <label for="d1-dev-project-path">{{ tApp('datasource.form.d1.devProjectPath') }}</label>
          <input
            id="d1-dev-project-path"
            v-model="form.d1DevProjectPath"
            :placeholder="tApp('datasource.form.d1.devProjectPathPlaceholder')"
            v-bind="textInputAttrs"
          />
          <p class="meta">{{ tApp('datasource.form.d1.devProjectPathHint') }}</p>
        </div>
        <div v-if="showD1DatabaseSelector && d1CreateDatabaseOpen" class="field span-2">
          <label for="d1-create-database-name">{{ tApp('datasource.form.d1.newDatabaseNameLabel') }}</label>
          <div class="console-actions">
            <input
              id="d1-create-database-name"
              v-model="d1CreateDatabaseName"
              :placeholder="tApp('datasource.form.d1.newDatabaseNamePlaceholder')"
              :disabled="d1CreateDatabaseLoading"
              v-bind="textInputAttrs"
            />
            <button class="btn secondary" type="button" :disabled="d1CreateDatabaseLoading" @click="createD1Database">
              {{ tApp('datasource.form.d1.createDatabase') }}
            </button>
            <button class="btn ghost" type="button" :disabled="d1CreateDatabaseLoading" @click="cancelCreateD1Database">
              {{ tApp('datasource.form.d1.cancelCreateDatabase') }}
            </button>
          </div>
        </div>
        <div v-if="isMongo" class="field span-2">
          <label for="mongo-conn-mode">{{ tApp('datasource.form.mongoConnection') }}</label>
          <select id="mongo-conn-mode" v-model="form.mongoMode">
            <option value="userpass">{{ tApp('datasource.form.mongoConnection.userpass') }}</option>
            <option value="uri">{{ tApp('datasource.form.mongoConnection.uri') }}</option>
          </select>
        </div>
        <div v-if="isSQL" class="field span-2">
          <label for="sql-conn-mode">{{ tApp('datasource.form.sqlConnection') }}</label>
          <select id="sql-conn-mode" v-model="form.sqlMode">
            <option value="userpass">{{ tApp('datasource.form.sqlConnection.userpass') }}</option>
            <option value="uri">{{ tApp('datasource.form.sqlConnection.uri') }}</option>
          </select>
        </div>
        <div v-if="isSQL && form.sqlMode === 'uri'" class="field span-2">
          <label for="sql-uri">{{ tApp('datasource.form.sqlUri') }}</label>
          <input
            id="sql-uri"
            v-model="form.sqlUri"
            :placeholder="form.type === 'postgresql'
              ? tApp('datasource.form.sqlUriPlaceholder.postgresql')
              : tApp('datasource.form.sqlUriPlaceholder.mysql')"
            :class="fieldClass('sqlUri')"
            v-bind="textInputAttrs"
          />
        </div>
        <div v-if="isSQL && form.type === 'postgresql'" class="field span-2">
          <label for="pg-ssl-enabled" class="checkbox-inline-label">
            <input id="pg-ssl-enabled" type="checkbox" v-model="form.pgSslEnabled" />
            {{ tApp('datasource.form.postgresSslEnabled') }}
          </label>
          <p class="meta">{{ tApp('datasource.form.postgresSslEnabledHint') }}</p>
        </div>
        <div v-if="isSQL && form.type === 'postgresql' && form.pgSslEnabled" class="field span-2">
          <label for="pg-ssl-certificate-file">{{ tApp('datasource.form.postgresSslCertificate') }}</label>
          <input
            id="pg-ssl-certificate-file"
            type="file"
            accept=".crt,.cer,.pem,.txt"
            @change="handlePostgresCertificateFile"
          />
          <p class="meta">{{ tApp('datasource.form.postgresSslCertificateHint') }}</p>
          <p v-if="form.pgSslRootCert && pgSslStoredCertificatePath" class="meta pg-cert-meta pg-cert-meta-success">
            <span>{{ tApp('datasource.form.postgresSslCertificateStoredPrefix') }}</span>
            <a href="#" class="pg-cert-link" @click.prevent="showPostgresCertificatePath">{{ pgSslDisplayedCertificateName }}</a>
            <span>{{ tApp('datasource.form.postgresSslCertificateStoredSuffix') }}</span>
          </p>
          <p v-else-if="form.pgSslRootCert" class="meta">
            {{
              form.pgSslCertFileName
                ? tApp('datasource.form.postgresSslCertificateSelected', { name: form.pgSslCertFileName })
                : tApp('datasource.form.postgresSslCertificateStored')
            }}
          </p>
        </div>
        <div v-if="isSQL && form.type === 'mysql'" class="field span-2">
          <label for="mysql-ssl-enabled" class="checkbox-inline-label">
            <input id="mysql-ssl-enabled" type="checkbox" v-model="form.mysqlSslEnabled" />
            {{ tApp('datasource.form.mysqlSslEnabled') }}
          </label>
          <p class="meta">{{ tApp('datasource.form.mysqlSslEnabledHint') }}</p>
        </div>
        <div v-if="isSQL && form.type === 'mysql' && form.mysqlSslEnabled" class="field span-2">
          <label for="mysql-ssl-certificate-file">{{ tApp('datasource.form.mysqlSslCertificate') }}</label>
          <input
            id="mysql-ssl-certificate-file"
            type="file"
            accept=".crt,.cer,.pem,.txt"
            @change="handleMySQLCertificateFile"
          />
          <p class="meta">{{ tApp('datasource.form.mysqlSslCertificateHint') }}</p>
          <p v-if="form.mysqlSslRootCert && mysqlSslStoredCertificatePath" class="meta pg-cert-meta pg-cert-meta-success">
            <span>{{ tApp('datasource.form.mysqlSslCertificateStoredPrefix') }}</span>
            <a href="#" class="pg-cert-link" @click.prevent="showMySQLCertificatePath">{{ mysqlSslDisplayedCertificateName }}</a>
            <span>{{ tApp('datasource.form.mysqlSslCertificateStoredSuffix') }}</span>
          </p>
          <p v-else-if="form.mysqlSslRootCert" class="meta">
            {{
              form.mysqlSslCertFileName
                ? tApp('datasource.form.mysqlSslCertificateSelected', { name: form.mysqlSslCertFileName })
                : tApp('datasource.form.mysqlSslCertificateStored')
            }}
          </p>
        </div>
        <div v-if="isMongo && form.mongoMode === 'uri'" class="field span-2">
          <label for="mongo-uri">{{ tApp('datasource.form.mongoUri') }}</label>
          <input
            id="mongo-uri"
            v-model="form.mongoUri"
            placeholder="mongodb://user:pass@host1:27017,host2:27017/db?replicaSet=rs0&authSource=admin"
            v-bind="textInputAttrs"
          />
        </div>
        <div v-if="isMongo && form.mongoMode === 'userpass'" class="field">
          <label for="mongo-replicaset">{{ tApp('datasource.form.replicaSetOptional') }}</label>
          <input id="mongo-replicaset" v-model="form.mongoReplicaSet" placeholder="rs0" v-bind="textInputAttrs" />
        </div>
        <div v-if="isMongo && form.mongoMode === 'userpass'" class="field span-2">
          <label for="mongo-hosts">{{ tApp('datasource.form.hosts') }}</label>
          <input
            id="mongo-hosts"
            v-model="form.mongoHosts"
            placeholder="host1:27017,host2:27018,host3:27019"
            v-bind="textInputAttrs"
          />
        </div>
        <div v-if="isMongo" class="field span-2">
          <label for="mongo-tls" class="checkbox-inline-label">
            <input id="mongo-tls" type="checkbox" v-model="form.mongoTls" />
            {{ tApp('datasource.form.mongoSslEnabled') }}
          </label>
          <p class="meta">{{ tApp('datasource.form.mongoSslEnabledHint') }}</p>
        </div>
        <div v-if="isMongo && form.mongoTls" class="field span-2">
          <label for="mongo-ssl-certificate-file">{{ tApp('datasource.form.mongoSslCertificate') }}</label>
          <input
            id="mongo-ssl-certificate-file"
            type="file"
            accept=".crt,.cer,.pem,.txt"
            @change="handleMongoCertificateFile"
          />
          <p class="meta">{{ tApp('datasource.form.mongoSslCertificateHint') }}</p>
          <p v-if="form.mongoSslRootCert && mongoSslStoredCertificatePath" class="meta pg-cert-meta pg-cert-meta-success">
            <span>{{ tApp('datasource.form.mongoSslCertificateStoredPrefix') }}</span>
            <a href="#" class="pg-cert-link" @click.prevent="showMongoCertificatePath">{{ mongoSslDisplayedCertificateName }}</a>
            <span>{{ tApp('datasource.form.mongoSslCertificateStoredSuffix') }}</span>
          </p>
          <p v-else-if="form.mongoSslRootCert" class="meta">
            {{
              form.mongoSslCertFileName
                ? tApp('datasource.form.mongoSslCertificateSelected', { name: form.mongoSslCertFileName })
                : tApp('datasource.form.mongoSslCertificateStored')
            }}
          </p>
        </div>
        <div v-if="isChroma" class="field">
          <label for="chromadb-scheme">{{ tApp('datasource.form.chromadb.scheme') }}</label>
          <select id="chromadb-scheme" v-model="form.chromaScheme" :class="fieldClass('chromaScheme')">
            <option value="http">http</option>
            <option value="https">https</option>
          </select>
        </div>
        <div v-if="isChroma" class="field">
          <label for="chromadb-tenant">{{ tApp('datasource.form.chromadb.tenant') }}</label>
          <input
            id="chromadb-tenant"
            v-model="form.chromaTenant"
            :placeholder="tApp('datasource.form.chromadb.tenantPlaceholder')"
            v-bind="textInputAttrs"
          />
        </div>
        <div v-if="isChroma" class="field">
          <label for="chromadb-database">{{ tApp('datasource.form.chromadb.database') }}</label>
          <input
            id="chromadb-database"
            v-model="form.chromaDatabase"
            :placeholder="tApp('datasource.form.chromadb.databasePlaceholder')"
            v-bind="textInputAttrs"
          />
        </div>
        <div v-if="isChroma" class="field">
          <label for="chromadb-api-token">{{ tApp('datasource.form.chromadb.apiTokenOptional') }}</label>
          <input
            id="chromadb-api-token"
            v-model="form.chromaApiToken"
            type="password"
            autocomplete="off"
            v-bind="textInputAttrs"
          />
          <p class="meta">{{ tApp('datasource.form.chromadb.apiTokenHint') }}</p>
        </div>
        <div v-if="(!isMongo || form.mongoMode === 'userpass') && (!isSQL || form.sqlMode === 'userpass') && !isDynamo && !isD1">
          <label for="ds-host">{{ tApp('common.host') }}</label>
          <input id="ds-host" v-model="form.host" placeholder="127.0.0.1" :class="fieldClass('host')" v-bind="textInputAttrs" />
        </div>
        <div v-if="(!isMongo || form.mongoMode === 'userpass') && (!isSQL || form.sqlMode === 'userpass') && !isDynamo && !isD1">
          <label for="ds-port">{{ tApp('common.port') }}</label>
          <input id="ds-port" v-model="form.port" :placeholder="portPlaceholder" :class="fieldClass('port')" v-bind="textInputAttrs" />
        </div>
        <div v-if="(!isMongo || form.mongoMode === 'userpass') && (!isSQL || form.sqlMode === 'userpass') && !isDynamo && !isD1 && !isChroma" class="field">
          <label for="ds-username">{{ tApp('common.username') }}</label>
          <input id="ds-username" v-model="form.username" placeholder="root" :class="fieldClass('username')" v-bind="textInputAttrs" />
        </div>
        <div v-if="(!isMongo || form.mongoMode === 'userpass') && (!isSQL || form.sqlMode === 'userpass') && !isDynamo && !isD1 && !isChroma" class="field">
          <label for="ds-password">{{ tApp('common.password') }}</label>
          <select
            v-if="showPasswordSecretRef"
            id="ds-password-secret-mode"
            class="secret-source-select"
            v-model="form.passwordSecretMode"
            @change="handlePasswordSecretModeChange"
          >
            <option value="manual">{{ tApp('datasource.form.secret.modeManual') }}</option>
            <option value="existing">{{ tApp('datasource.form.secret.modeExisting') }}</option>
          </select>
          <div v-if="!usePasswordSecretRef" class="password-toggle-wrapper">
            <input id="ds-password" v-model="form.password" :type="showPassword ? 'text' : 'password'" v-bind="textInputAttrs" />
            <button type="button" class="password-toggle-btn" @click="showPassword = !showPassword" tabindex="-1">
              <EyeOff v-if="showPassword" :size="16" />
              <Eye v-else :size="16" />
            </button>
          </div>
        </div>
        <div v-if="usePasswordSecretRef" class="field span-2 secret-ref-fields">
          <p class="meta">{{ tApp('datasource.form.secret.hint') }}</p>
          <div class="secret-ref-grid">
            <div class="field">
              <label for="ds-password-secret-provider">{{ tApp('datasource.form.secret.provider') }}</label>
              <select
                id="ds-password-secret-provider"
                v-model="form.passwordSecretProviderId"
                :class="fieldClass('passwordSecretProviderId')"
              >
                <option value="" disabled>{{ tApp('datasource.form.secret.providerPlaceholder') }}</option>
                <option v-for="provider in secretProviders" :key="provider.id" :value="provider.id">
                  {{ provider.name || provider.id }}
                </option>
              </select>
            </div>
            <div class="field">
              <label for="ds-password-secret-key">{{ tApp('datasource.form.secret.key') }}</label>
              <input
                id="ds-password-secret-key"
                v-model="form.passwordSecretKey"
                :placeholder="tApp('datasource.form.secret.keyPlaceholder')"
                :class="fieldClass('passwordSecretKey')"
                v-bind="textInputAttrs"
              />
            </div>
            <div class="field">
              <label for="ds-password-secret-field">{{ tApp('datasource.form.secret.field') }}</label>
              <input
                id="ds-password-secret-field"
                v-model="form.passwordSecretField"
                :placeholder="tApp('datasource.form.secret.fieldPlaceholder')"
                v-bind="textInputAttrs"
              />
            </div>
            <div class="field">
              <label for="ds-password-secret-version">{{ tApp('datasource.form.secret.version') }}</label>
              <input
                id="ds-password-secret-version"
                v-model="form.passwordSecretVersion"
                :placeholder="tApp('datasource.form.secret.versionPlaceholder')"
                v-bind="textInputAttrs"
              />
            </div>
          </div>
        </div>
        <div v-if="isSQL && form.sqlMode === 'userpass'" class="field span-2">
          <label for="ds-database">{{ tApp('common.database') }}</label>
          <input
            id="ds-database"
            v-model="form.database"
            :placeholder="databasePlaceholder"
            :class="fieldClass('database')"
            v-bind="textInputAttrs"
          />
        </div>
        <div v-if="isMongo" class="field">
          <label for="ds-database">{{ tApp('common.database') }}</label>
          <input id="ds-database" v-model="form.database" placeholder="default" v-bind="textInputAttrs" />
        </div>
        <div v-if="isMongo" class="field">
          <label for="ds-authsource">{{ tApp('datasource.form.authSource') }}</label>
          <input id="ds-authsource" v-model="form.authSource" placeholder="admin" v-bind="textInputAttrs" />
        </div>
        <div v-if="!isMongo && !isDynamo && !isD1 && !isChroma && (!isSQL || form.sqlMode === 'userpass')" class="field span-2">
          <label for="ds-options">{{ tApp('datasource.form.optionsJson') }}</label>
          <textarea id="ds-options" v-model="form.options" placeholder='{"sslmode":"disable"}' v-bind="textInputAttrs"></textarea>
        </div>
      </div>

      <p class="meta" id="form-hint">{{ hint }}</p>

      <details v-if="installGuide" class="install-details">
        <summary>{{ tApp('datasource.form.quickInstallDocker') }}</summary>
        <div class="install-grid">
          <div class="install-block">
            <div class="meta">{{ tApp('datasource.form.install.run') }}</div>
            <pre class="json">{{ installGuide.run }}</pre>
            <button class="btn ghost mini" type="button" @click="copyInstall(installGuide.run)">{{ tApp('common.copy') }}</button>
          </div>
          <div class="install-block">
            <div class="meta">{{ tApp('datasource.form.install.stopRemove') }}</div>
            <pre class="json">{{ installGuide.remove }}</pre>
            <button class="btn ghost mini" type="button" @click="copyInstall(installGuide.remove)">{{ tApp('common.copy') }}</button>
          </div>
          <div v-if="installGuide.connect" class="install-block">
            <div class="meta">{{ tApp('datasource.form.install.connect') }}</div>
            <pre class="json">{{ installGuide.connect }}</pre>
            <button class="btn ghost mini" type="button" @click="copyInstall(installGuide.connect)">{{ tApp('common.copy') }}</button>
          </div>
        </div>
      </details>

      <div class="console-actions">
        <div class="test-connection-block" data-testid="datasource-form-test-connection">
          <div v-if="d1CreateStatusText" class="test-connection-status" :class="d1CreateStatusClass" data-testid="d1-create-database-status">
            <span class="status" :class="d1CreateStatusClass">{{ d1CreateStatusText }}</span>
            <span v-if="d1CreateStatusDetail" class="test-connection-detail">{{ d1CreateStatusDetail }}</span>
          </div>
          <div v-if="testStatusText" class="test-connection-status" :class="testStatusClass">
            <span class="status" :class="testStatusClass">{{ testStatusText }}</span>
            <span v-if="testStatusDetail" class="test-connection-detail">{{ testStatusDetail }}</span>
          </div>
          <button class="btn test-connection-btn" type="button" @click="testConnection">
            {{ tApp('datasource.form.testConnection') }}
          </button>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { Eye, EyeOff } from 'lucide-vue-next'
import { useDatasourceFormView } from './datasource-form/useDatasourceFormView'
import DatasourceTypeSelect from '@/components/DatasourceTypeSelect.vue'
import { tApp } from '@/modules/i18n/appI18n'

const showPassword = ref(false)
const showDynamoSecret = ref(false)

const {
  store,
  form,
  errors,
  testStatusText,
  testStatusDetail,
  testStatusClass,
  d1CreateStatusText,
  d1CreateStatusDetail,
  d1CreateStatusClass,
  dataSourceTypeOptions,
  isMongo,
  isDynamo,
  isDynamoSSO,
  isD1,
  isChroma,
  isSQL,
  secretProviders,
  showPasswordSecretRef,
  usePasswordSecretRef,
  handlePasswordSecretModeChange,
  pgSslStoredCertificatePath,
  pgSslDisplayedCertificateName,
  mysqlSslStoredCertificatePath,
  mysqlSslDisplayedCertificateName,
  mongoSslStoredCertificatePath,
  mongoSslDisplayedCertificateName,
  formTitle,
  portPlaceholder,
  databasePlaceholder,
  hint,
  installGuide,
  copyInstall,
  copyDynamoSecretAccessKey,
  copyDynamoSessionToken,
  maskedDynamoSecretAccessKey,
  maskedDynamoSessionToken,
  importDynamoCredentialsFromFile,
  dynamoSSOProfiles,
  dynamoSSOProfilesLoading,
  dynamoSSOConfigApplyLoading,
  dynamoSSOOAuthLoading,
  dynamoSSOVerified,
  dynamoSSOConnected,
  dynamoSSOConfigEndpoint,
  dynamoSSOHasConfigEndpoint,
  loadDynamoSSOProfiles,
  applyDynamoSSOConfigPath,
  dynamoSSOOAuthAuthorize,
  markDynamoRegionAsManual,
  importPostgresCertificateFromFile,
  importMySQLCertificateFromFile,
  importMongoCertificateFromFile,
  showPostgresCertificatePath,
  showMySQLCertificatePath,
  showMongoCertificatePath,
  fieldClass,
  d1Accounts,
  d1Databases,
  d1OAuthLoading,
  d1DatabasesLoading,
  d1CreateDatabaseOpen,
  d1CreateDatabaseLoading,
  d1CreateDatabaseName,
  d1DatabaseCreateOptionValue,
  d1OAuthAuthenticated,
  d1OAuthVerified,
  d1ConnectionEstablished,
  d1CanCreateDatabase,
  d1WranglerMissing,
  showD1DatabaseSelector,
  d1OAuthLogin,
  handleD1AccountSelection,
  handleD1DatabaseSelection,
  createD1Database,
  cancelCreateD1Database,
  markD1SupportDevTouched,
  save,
  testConnection,
  cancel,
} = useDatasourceFormView()

const textInputAttrs = {
  autocapitalize: 'off',
  autocorrect: 'off',
  spellcheck: 'false',
}

const handleDynamoCredentialsFile = async (event: Event) => {
  const target = event.target as HTMLInputElement | null
  const files = (target as any)?.files as File[] | FileList | undefined
  const file = files && (files as any)[0]
  if (!file) return
  await importDynamoCredentialsFromFile(file)
  if (target) target.value = ''
}

const handlePostgresCertificateFile = async (event: Event) => {
  const target = event.target as HTMLInputElement | null
  const files = (target as any)?.files as File[] | FileList | undefined
  const file = files && (files as any)[0]
  if (!file) return
  await importPostgresCertificateFromFile(file)
  if (target) target.value = ''
}

const handleMySQLCertificateFile = async (event: Event) => {
  const target = event.target as HTMLInputElement | null
  const files = (target as any)?.files as File[] | FileList | undefined
  const file = files && (files as any)[0]
  if (!file) return
  await importMySQLCertificateFromFile(file)
  if (target) target.value = ''
}

const handleMongoCertificateFile = async (event: Event) => {
  const target = event.target as HTMLInputElement | null
  const files = (target as any)?.files as File[] | FileList | undefined
  const file = files && (files as any)[0]
  if (!file) return
  await importMongoCertificateFromFile(file)
  if (target) target.value = ''
}
</script>
