package com.example.service

import android.content.Context
import androidx.work.*
import com.example.ButlerApplication
import com.example.data.repository.SyncBatchResult
import kotlinx.coroutines.flow.first
import java.util.concurrent.TimeUnit

/** Persistent recovery/retry; the listener's debounce is only a latency aid. */
class NotificationSyncWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result {
        val app = applicationContext as ButlerApplication
        val config = app.credentialStore.state.value
        if (!config.isPaired || config.isSyncPaused ||
            !ButlerNotificationListenerService.isNotificationAccessGranted(app)) return Result.success()
        app.syncRepository.ensureDefaultAppRulesSeeded()
        repeat(20) {
            when (val result = app.syncRepository.syncPendingQueueBatch()) {
                is SyncBatchResult.Failure -> return if (result.retryScheduled) Result.retry() else Result.success()
                is SyncBatchResult.Idle -> return if (app.syncRepository.pendingCount.first() > 0) Result.retry() else Result.success()
                is SyncBatchResult.Success -> Unit
            }
        }
        return if (app.syncRepository.pendingCount.first() > 0) Result.retry() else Result.success()
    }

    companion object {
        private val constraints = Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build()
        fun enqueue(context: Context) {
            val request = OneTimeWorkRequestBuilder<NotificationSyncWorker>()
                .setConstraints(constraints).setInitialDelay(2, TimeUnit.SECONDS)
                .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 15, TimeUnit.SECONDS).build()
            WorkManager.getInstance(context).enqueueUniqueWork("butler-notification-sync", ExistingWorkPolicy.KEEP, request)
        }
        fun recoverPeriodically(context: Context) {
            WorkManager.getInstance(context).enqueueUniquePeriodicWork("butler-notification-recovery",
                ExistingPeriodicWorkPolicy.KEEP,
                PeriodicWorkRequestBuilder<NotificationSyncWorker>(15, TimeUnit.MINUTES).setConstraints(constraints).build())
        }
    }
}
