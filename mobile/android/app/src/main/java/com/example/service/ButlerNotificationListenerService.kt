package com.example.service

import android.app.Notification
import android.content.ComponentName
import android.content.Context
import android.os.Build
import android.os.Bundle
import android.os.Parcelable
import android.provider.Settings
import android.service.notification.NotificationListenerService
import android.service.notification.StatusBarNotification
import androidx.core.app.NotificationManagerCompat
import com.example.ButlerApplication
import com.example.domain.RawNotificationSnapshot
import com.example.model.ConversationMessageDto
import com.example.model.NotificationVisibilityLevel
import com.example.model.ProgressDto
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/**
 * Standard Android [NotificationListenerService] implementation for Butler Companion.
 *
 * Strictly adheres to Issue #57 boundaries:
 * - Only observes notifications after explicit owner consent in Android System Settings.
 * - On initial connection (`onListenerConnected`), performs a single snapshot of currently active
 *   notifications marked with `is_initial_snapshot = true`.
 * - Never invokes notification actions, never replies, never dismisses user notifications,
 *   and never writes notification content to Logcat.
 */
class ButlerNotificationListenerService : NotificationListenerService() {

    private val serviceScope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private var debouncedSyncJob: Job? = null
    private var initialSnapshotTakenForSession = false

    override fun onListenerConnected() {
        super.onListenerConnected()
        _isListenerConnected.value = true
        val app = applicationContext as? ButlerApplication ?: ButlerApplication.instance ?: return

        serviceScope.launch {
            app.syncRepository.logDiagnostic(
                level = "INFO",
                stage = "LISTENER",
                code = "LISTENER_CONNECTED",
                summaryRu = "Служба NotificationListenerService подключена системой Android"
            )
            if (!initialSnapshotTakenForSession) {
                initialSnapshotTakenForSession = true
                captureInitialActiveNotificationsSnapshot(app)
            }
            scheduleDebouncedBatchSync(app)
        }
    }

    override fun onListenerDisconnected() {
        super.onListenerDisconnected()
        _isListenerConnected.value = false
        initialSnapshotTakenForSession = false
        val app = applicationContext as? ButlerApplication ?: ButlerApplication.instance ?: return
        serviceScope.launch {
            app.syncRepository.logDiagnostic(
                level = "WARN",
                stage = "LISTENER",
                code = "LISTENER_DISCONNECTED",
                summaryRu = "Служба уведомлений отключена системой или разрешение отозвано"
            )
        }
    }

    override fun onNotificationPosted(sbn: StatusBarNotification?) {
        super.onNotificationPosted(sbn)
        if (sbn == null) return
        val app = applicationContext as? ButlerApplication ?: ButlerApplication.instance ?: return

        serviceScope.launch {
            if (!captureEnabled(app) || !app.syncRepository.shouldCapturePackage(sbn.packageName)) return@launch
            val snapshot = extractRawSnapshot(sbn, applicationContext)
            val emitted = app.syncRepository.ingestRawNotification(
                snapshot = snapshot,
                isInitialSnapshot = false
            )
            if (emitted != null) {
                scheduleDebouncedBatchSync(app)
            }
        }
    }

    override fun onNotificationRemoved(sbn: StatusBarNotification?) {
        super.onNotificationRemoved(sbn)
        if (sbn == null) return
        val app = applicationContext as? ButlerApplication ?: ButlerApplication.instance ?: return

        val key = computeStableNotificationKey(sbn)
        val pkg = sbn.packageName ?: return
        val channelId = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            sbn.notification?.channelId
        } else null
        val postedAt = sbn.postTime

        serviceScope.launch {
            if (!captureEnabled(app) || !app.syncRepository.shouldCapturePackage(pkg)) return@launch
            val emitted = app.syncRepository.ingestNotificationRemoved(
                notificationKey = key,
                packageName = pkg,
                channelId = channelId,
                postedAtEpochMs = postedAt
            )
            if (emitted != null) {
                scheduleDebouncedBatchSync(app)
            }
        }
    }

    override fun onDestroy() {
        super.onDestroy()
        _isListenerConnected.value = false
        serviceScope.cancel()
    }

    private suspend fun captureInitialActiveNotificationsSnapshot(app: ButlerApplication) {
        if (!captureEnabled(app)) return
        val activeList = runCatching { activeNotifications }.getOrNull() ?: return
        var capturedCount = 0
        for (sbn in activeList) {
            if (sbn == null) continue
            if (!captureEnabled(app)) return
            if (!app.syncRepository.shouldCapturePackage(sbn.packageName)) continue
            val snapshot = extractRawSnapshot(sbn, applicationContext)
            val entity = app.syncRepository.ingestRawNotification(
                snapshot = snapshot,
                isInitialSnapshot = true
            )
            if (entity != null) {
                capturedCount++
            }
        }
        if (capturedCount > 0) {
            app.syncRepository.logDiagnostic(
                level = "INFO",
                stage = "LISTENER",
                code = "INITIAL_ACTIVE_SNAPSHOT",
                summaryRu = "Снят однократный срез активных уведомлений при подключении: $capturedCount шт."
            )
        }
    }

    private fun captureEnabled(app: ButlerApplication): Boolean {
        val config = app.credentialStore.state.value
        return config.isPaired && !config.isSyncPaused && !config.isTokenRevoked && isNotificationAccessGranted(app)
    }

    private fun scheduleDebouncedBatchSync(app: ButlerApplication) {
        NotificationSyncWorker.enqueue(app)
        debouncedSyncJob?.cancel()
        debouncedSyncJob = serviceScope.launch {
            delay(1_200L)
            app.syncRepository.syncPendingQueueBatch()
        }
    }

    companion object {
        private val _isListenerConnected = MutableStateFlow(false)
        val isListenerConnected: StateFlow<Boolean> = _isListenerConnected.asStateFlow()

        fun isNotificationAccessGranted(context: Context): Boolean {
            return try {
                val enabledPackages = NotificationManagerCompat.getEnabledListenerPackages(context)
                if (context.packageName in enabledPackages) return true

                val flat = Settings.Secure.getString(
                    context.contentResolver,
                    "enabled_notification_listeners"
                ).orEmpty()
                val myComponent = ComponentName(context, ButlerNotificationListenerService::class.java).flattenToString()
                flat.contains(myComponent) || flat.contains(context.packageName)
            } catch (_: Exception) {
                false
            }
        }

        fun computeStableNotificationKey(sbn: StatusBarNotification): String {
            return sbn.key ?: "${sbn.packageName}|${sbn.id}|${sbn.tag.orEmpty()}"
        }

        fun extractRawSnapshot(sbn: StatusBarNotification, context: Context? = null): RawNotificationSnapshot {
            val pkg = sbn.packageName ?: "unknown"
            val resolvedAppLabel = context?.let { ctx ->
                runCatching {
                    val pm = ctx.packageManager
                    val appInfo = pm.getApplicationInfo(pkg, 0)
                    pm.getApplicationLabel(appInfo).toString()
                }.getOrNull()
            }

            val notification = sbn.notification
            val extras: Bundle? = notification?.extras

            val title = extras?.getCharSequence(Notification.EXTRA_TITLE)?.toString()
                ?: extras?.getCharSequence(Notification.EXTRA_TITLE_BIG)?.toString()
            val text = extras?.getCharSequence(Notification.EXTRA_TEXT)?.toString()
            val bigText = extras?.getCharSequence(Notification.EXTRA_BIG_TEXT)?.toString()
                ?: extras?.getCharSequenceArray(Notification.EXTRA_TEXT_LINES)?.joinToString("\n")
            val subText = extras?.getCharSequence(Notification.EXTRA_SUB_TEXT)?.toString()
            val summaryText = extras?.getCharSequence(Notification.EXTRA_SUMMARY_TEXT)?.toString()
            val infoText = extras?.getCharSequence(Notification.EXTRA_INFO_TEXT)?.toString()
            val conversationTitle = extras?.getCharSequence(Notification.EXTRA_CONVERSATION_TITLE)?.toString()

            val messages = extractMessagingStyleMessages(extras)

            val progressMax = extras?.getInt(Notification.EXTRA_PROGRESS_MAX, 0) ?: 0
            val progressCurrent = extras?.getInt(Notification.EXTRA_PROGRESS, 0) ?: 0
            val progressIndeterminate = extras?.getBoolean(Notification.EXTRA_PROGRESS_INDETERMINATE, false) ?: false
            val progress = if (progressMax > 0 || progressIndeterminate) {
                ProgressDto(
                    current = progressCurrent,
                    max = progressMax,
                    indeterminate = progressIndeterminate
                )
            } else null

            val flags = notification?.flags ?: 0
            val isGroupSummary = (flags and Notification.FLAG_GROUP_SUMMARY) != 0
            val isOngoing = sbn.isOngoing || (flags and Notification.FLAG_ONGOING_EVENT) != 0

            val visibility = when (notification?.visibility) {
                Notification.VISIBILITY_PUBLIC -> NotificationVisibilityLevel.PUBLIC
                Notification.VISIBILITY_PRIVATE -> NotificationVisibilityLevel.PRIVATE
                Notification.VISIBILITY_SECRET -> NotificationVisibilityLevel.SECRET
                else -> NotificationVisibilityLevel.UNKNOWN
            }

            val channelId = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                notification?.channelId
            } else null

            // Check Android 15+ OTP redaction flags if present in extras or publicVersion
            val redactedByOs = (notification?.visibility == Notification.VISIBILITY_SECRET) ||
                (extras?.containsKey("android.redactOtp") == true)

            return RawNotificationSnapshot(
                notificationKey = computeStableNotificationKey(sbn),
                packageName = pkg,
                appLabelHint = resolvedAppLabel,
                channelId = channelId,
                postedAtEpochMs = sbn.postTime,
                observedAtEpochMs = System.currentTimeMillis(),
                category = notification?.category,
                visibility = visibility,
                isGroupSummary = isGroupSummary,
                groupKey = sbn.groupKey,
                isOngoing = isOngoing,
                isClearable = sbn.isClearable,
                platformRedactedOtp = redactedByOs,
                title = title,
                text = text,
                bigText = bigText,
                subText = subText,
                summaryText = summaryText,
                infoText = infoText,
                conversationTitle = conversationTitle,
                messages = messages,
                progress = progress
            )
        }

        @Suppress("DEPRECATION")
        private fun extractMessagingStyleMessages(extras: Bundle?): List<ConversationMessageDto> {
            if (extras == null) return emptyList()
            val parcelables: Array<Parcelable>? = try {
                extras.getParcelableArray(Notification.EXTRA_MESSAGES)
            } catch (_: Exception) {
                null
            }
            if (parcelables.isNullOrEmpty()) return emptyList()

            val result = mutableListOf<ConversationMessageDto>()
            for (item in parcelables) {
                val bundle = item as? Bundle ?: continue
                val msgText = bundle.getCharSequence("text")?.toString() ?: continue
                val sender = bundle.getCharSequence("sender")?.toString()
                    ?: if (Build.VERSION.SDK_INT >= 28) (bundle.getParcelable<Parcelable>("sender_person") as? android.app.Person)?.name?.toString() else null
                val time = bundle.getLong("time", System.currentTimeMillis())
                result.add(
                    ConversationMessageDto(
                        sender = sender,
                        text = msgText,
                        timestampEpochMs = time
                    )
                )
            }
            return result
        }
    }
}
