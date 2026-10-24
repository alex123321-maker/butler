package com.example

import android.app.Application
import com.example.data.local.ButlerDatabase
import com.example.data.repository.ButlerSyncRepository
import com.example.security.DeviceCredentialStore
import com.example.security.KeystoreCryptoManager
import com.example.service.NetworkConnectivityMonitor
import com.example.service.NotificationSyncWorker
import com.example.service.ButlerNotificationListenerService
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch

class ButlerApplication : Application() {

    val appScope = CoroutineScope(SupervisorJob() + Dispatchers.IO)

    lateinit var cryptoManager: KeystoreCryptoManager
        private set

    lateinit var credentialStore: DeviceCredentialStore
        private set

    lateinit var database: ButlerDatabase
        private set

    lateinit var syncRepository: ButlerSyncRepository
        private set

    lateinit var networkMonitor: NetworkConnectivityMonitor
        private set

    override fun onCreate() {
        super.onCreate()
        instance = this

        cryptoManager = KeystoreCryptoManager()
        credentialStore = DeviceCredentialStore(this, cryptoManager)
        database = ButlerDatabase.getInstance(this)
        syncRepository = ButlerSyncRepository(
            dao = database.butlerDao(),
            credentialStore = credentialStore,
            cryptoManager = cryptoManager,
            notificationAccessGranted = { ButlerNotificationListenerService.isNotificationAccessGranted(this) },
            defaultSmsPackage = { android.provider.Telephony.Sms.getDefaultSmsPackage(this) }
        )
        networkMonitor = NetworkConnectivityMonitor(
            context = this,
            onNetworkRestored = {
                appScope.launch {
                    syncRepository.syncPendingQueueBatch()
                }
            }
        )

        appScope.launch {
            syncRepository.ensureDefaultAppRulesSeeded()
            NotificationSyncWorker.recoverPeriodically(this@ButlerApplication)
            NotificationSyncWorker.enqueue(this@ButlerApplication)
        }
    }

    companion object {
        @Volatile
        var instance: ButlerApplication? = null
            private set
    }
}
