package com.example.ui

import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.Context
import android.os.Build
import androidx.core.app.NotificationCompat
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import com.example.ButlerApplication
import com.example.R
import com.example.data.local.AppFilterRuleEntity
import com.example.data.local.DiagnosticLogEntity
import com.example.data.local.QueuedNotificationEntity
import com.example.data.repository.ButlerSyncRepository
import com.example.data.repository.PairingResult
import com.example.data.repository.SyncBatchResult
import com.example.domain.RawNotificationSnapshot
import com.example.model.AppFilterMode
import com.example.model.ConversationMessageDto
import com.example.model.LoopbackSimulationMode
import com.example.model.NotificationPayloadDto
import com.example.model.NotificationVisibilityLevel
import com.example.security.DeviceConfigurationState
import com.example.security.DeviceCredentialStore
import com.example.security.TransportPolicyValidator
import com.example.security.TransportValidationResult
import com.example.service.ButlerNotificationListenerService
import com.example.service.NetworkConnectivityMonitor
import com.example.service.SystemEnvironmentSnapshot
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch

enum class ButlerTab {
    DASHBOARD,
    PAIRING,
    FILTERS,
    QUEUE
}

data class QueueMetricsSummary(
    val pendingCount: Int = 0,
    val deliveredCount: Int = 0,
    val errorCount: Int = 0,
    val totalBytes: Long = 0L
)

class ButlerViewModel(
    private val appContext: Context,
    private val repository: ButlerSyncRepository,
    private val credentialStore: DeviceCredentialStore,
    private val networkMonitor: NetworkConnectivityMonitor
) : ViewModel() {

    private val _selectedTab = MutableStateFlow(ButlerTab.DASHBOARD)
    val selectedTab: StateFlow<ButlerTab> = _selectedTab.asStateFlow()

    private val _statusBannerMessage = MutableStateFlow<String?>(null)
    val statusBannerMessage: StateFlow<String?> = _statusBannerMessage.asStateFlow()

    private val _isSyncingNow = MutableStateFlow(false)
    val isSyncingNow: StateFlow<Boolean> = _isSyncingNow.asStateFlow()

    private val _hasNotificationAccess = MutableStateFlow(
        ButlerNotificationListenerService.isNotificationAccessGranted(appContext)
    )
    val hasNotificationAccess: StateFlow<Boolean> = _hasNotificationAccess.asStateFlow()

    val isListenerServiceConnected: StateFlow<Boolean> =
        ButlerNotificationListenerService.isListenerConnected

    val deviceConfig: StateFlow<DeviceConfigurationState> = credentialStore.state

    val environmentState: StateFlow<SystemEnvironmentSnapshot> = networkMonitor.environmentState

    val queueMetrics: StateFlow<QueueMetricsSummary> = combine(
        repository.pendingCount,
        repository.deliveredCount,
        repository.errorCount,
        repository.totalQueueBytes
    ) { pending, delivered, error, bytes ->
        QueueMetricsSummary(
            pendingCount = pending,
            deliveredCount = delivered,
            errorCount = error,
            totalBytes = bytes
        )
    }.stateIn(
        scope = viewModelScope,
        started = SharingStarted.WhileSubscribed(5_000),
        initialValue = QueueMetricsSummary()
    )

    val recentQueueEvents: StateFlow<List<QueuedNotificationEntity>> =
        repository.recentQueueEvents.stateIn(
            scope = viewModelScope,
            started = SharingStarted.WhileSubscribed(5_000),
            initialValue = emptyList()
        )

    val appFilterRules: StateFlow<List<AppFilterRuleEntity>> =
        repository.appFilterRules.stateIn(
            scope = viewModelScope,
            started = SharingStarted.WhileSubscribed(5_000),
            initialValue = emptyList()
        )

    val diagnosticLogs: StateFlow<List<DiagnosticLogEntity>> =
        repository.diagnosticLogs.stateIn(
            scope = viewModelScope,
            started = SharingStarted.WhileSubscribed(5_000),
            initialValue = emptyList()
        )

    private var syntheticCardCounter = 1001
    private var lastSyntheticKey: String? = null

    init {
        viewModelScope.launch {
            repository.ensureDefaultAppRulesSeeded()
        }
    }

    fun selectTab(tab: ButlerTab) {
        _selectedTab.value = tab
        refreshSystemPermissionAndNetworkState()
    }

    fun dismissBanner() {
        _statusBannerMessage.value = null
    }

    fun refreshSystemPermissionAndNetworkState() {
        _hasNotificationAccess.value =
            ButlerNotificationListenerService.isNotificationAccessGranted(appContext)
        networkMonitor.refreshSnapshot()
    }

    fun validateTransportUrl(url: String, allowCleartext: Boolean): TransportValidationResult {
        return TransportPolicyValidator.validate(url, allowCleartext)
    }

    // --- Sync & Test Actions ---

    fun setSyncPaused(paused: Boolean) {
        credentialStore.setSyncPaused(paused)
        viewModelScope.launch {
            repository.logDiagnostic(
                level = "INFO",
                stage = "QUEUE",
                code = if (paused) "SYNC_PAUSED" else "SYNC_RESUMED",
                summaryRu = if (paused) "Синхронизация приостановлена владельцем" else "Синхронизация возобновлена владельцем"
            )
            if (!paused) {
                triggerManualBatchSync()
            }
        }
    }

    fun setContentTransmissionEnabled(enabled: Boolean) {
        credentialStore.setContentTransmissionEnabled(enabled)
        _statusBannerMessage.value = if (enabled) {
            "Передача текста уведомлений включена (с защитой Android Keystore и маскировкой OTP)"
        } else {
            "Включён режим «Только метаданные»: заголовки и тексты уведомлений не сохраняются и не передаются"
        }
    }

    fun setMaskOtpAndBankCodes(enabled: Boolean) {
        credentialStore.setMaskOtpAndBankCodes(enabled)
    }

    fun setAppFilterMode(mode: AppFilterMode) {
        credentialStore.setAppFilterMode(mode)
    }

    fun setLoopbackSimulationMode(mode: LoopbackSimulationMode) {
        credentialStore.setLoopbackSimulationMode(mode)
        _statusBannerMessage.value = "Режим стенда контракта: ${mode.labelRu}"
    }

    fun togglePackageFilter(packageName: String, enabled: Boolean) {
        viewModelScope.launch {
            repository.setPackageEnabledForCurrentMode(packageName, enabled)
        }
    }

    fun addCustomPackageRule(packageName: String, appLabel: String) {
        viewModelScope.launch {
            repository.addCustomPackageFilterRule(packageName, appLabel)
            _statusBannerMessage.value = "Добавлено правило для пакета $packageName"
        }
    }

    fun triggerPingTest() {
        viewModelScope.launch {
            _isSyncingNow.value = true
            val (_, msg) = repository.pingServerStatus()
            _isSyncingNow.value = false
            _statusBannerMessage.value = msg
        }
    }

    fun triggerManualBatchSync() {
        viewModelScope.launch {
            _isSyncingNow.value = true
            val res = repository.syncPendingQueueBatch(ignoreBackoffTimers = true)
            _isSyncingNow.value = false
            _statusBannerMessage.value = when (res) {
                is SyncBatchResult.Success ->
                    "Синхронизировано: принято ${res.acceptedCount}, дублей ${res.duplicateCount}, конфликтов ${res.conflictCount} (${res.latencyMs} мс)"
                is SyncBatchResult.Idle -> res.reasonRu
                is SyncBatchResult.Failure -> "Ошибка доставки: ${res.messageRu}"
            }
        }
    }

    /**
     * Generates a synthetic notification event (including MessagingStyle & OTP sample),
     * stores it encrypted in Room, and attempts batch delivery to Butler.
     */
    fun sendSyntheticTestEvent(includeOtpSample: Boolean = false) {
        viewModelScope.launch {
            val now = System.currentTimeMillis()
            val idNum = syntheticCardCounter++
            val key = "0|org.telegram.messenger|$idNum|test|1000"
            lastSyntheticKey = key

            val sampleText = if (includeOtpSample) {
                "Ваш одноразовый код подтверждения входа: 482910. Никому не сообщайте код."
            } else {
                "Привет! Напоминаю про встречу по архитектуре Butler в 18:30 🚀"
            }

            val snapshot = RawNotificationSnapshot(
                notificationKey = key,
                packageName = if (includeOtpSample) "ru.sberbankmobile" else "org.telegram.messenger",
                channelId = if (includeOtpSample) "security_alerts" else "direct_messages",
                postedAtEpochMs = now,
                observedAtEpochMs = now,
                category = "msg",
                visibility = NotificationVisibilityLevel.PRIVATE,
                isGroupSummary = false,
                groupKey = "group_test_$idNum",
                title = if (includeOtpSample) "СберБанк Онлайн" else "Команда Butler",
                text = sampleText,
                conversationTitle = if (includeOtpSample) null else "Рабочий чат Butler",
                messages = if (includeOtpSample) {
                    emptyList()
                } else {
                    listOf(
                        ConversationMessageDto(
                            sender = "Алексей",
                            text = sampleText,
                            timestampEpochMs = now
                        )
                    )
                }
            )

            val created = repository.ingestRawNotification(
                snapshot = snapshot,
                isInitialSnapshot = false,
                bypassPauseForManualTest = true
            )
            if (created != null) {
                val syncRes = repository.syncPendingQueueBatch(ignoreBackoffTimers = true)
                _statusBannerMessage.value = when (syncRes) {
                    is SyncBatchResult.Success ->
                        "Тестовое событие (${created.changeType}) записано в Room (AES-256-GCM) и подтверждено сервером!"
                    is SyncBatchResult.Failure ->
                        "Тестовое событие сохранено в зашифрованную очередь Room (ожидание сети: ${syncRes.messageRu})"
                    is SyncBatchResult.Idle ->
                        "Тестовое событие сохранено в локальную зашифрованную очередь Room (${syncRes.reasonRu})"
                }
            }
        }
    }

    /**
     * Simulates updating and then removing the most recent synthetic notification card,
     * demonstrating `UPDATED` (new device_event_id + previous_device_event_id) and `REMOVED`.
     */
    fun triggerSyntheticUpdateAndRemoveLifecycle() {
        viewModelScope.launch {
            val now = System.currentTimeMillis()
            val targetKey = lastSyntheticKey ?: "0|org.telegram.messenger|${syntheticCardCounter++}|lifecycle|1000"
            lastSyntheticKey = targetKey

            val updatedSnapshot = RawNotificationSnapshot(
                notificationKey = targetKey,
                packageName = "org.telegram.messenger",
                channelId = "direct_messages",
                postedAtEpochMs = now,
                observedAtEpochMs = now,
                category = "msg",
                title = "Команда Butler (Обновлено)",
                text = "Время встречи перенесено на 19:00 — карточка обновлена (UPDATED)"
            )

            repository.ingestRawNotification(
                snapshot = updatedSnapshot,
                isInitialSnapshot = false,
                bypassPauseForManualTest = true
            )

            repository.ingestNotificationRemoved(
                notificationKey = targetKey,
                packageName = "org.telegram.messenger",
                channelId = "direct_messages",
                postedAtEpochMs = now,
                observedAtEpochMs = now + 50L
            )

            val syncRes = repository.syncPendingQueueBatch(ignoreBackoffTimers = true)
            _statusBannerMessage.value = when (syncRes) {
                is SyncBatchResult.Success ->
                    "Цепочка UPDATED → REMOVED с привязкой previous_device_event_id доставлена и подтверждена!"
                else ->
                    "Цепочка UPDATED → REMOVED записана в локальную зашифрованную очередь Room"
            }
        }
    }

    /**
     * Posts a real OS notification via Android's NotificationManager so that on a physical phone
     * (like Samsung Galaxy S10) with Notification Access enabled, the OS invokes
     * `ButlerNotificationListenerService.onNotificationPosted` end-to-end.
     */
    fun postRealSystemTestNotification() {
        try {
            val nm = appContext.getSystemService(Context.NOTIFICATION_SERVICE) as? NotificationManager ?: return
            val channelId = "butler_companion_test_channel"
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                val channel = NotificationChannel(
                    channelId,
                    "Butler Diagnostic Test Notifications",
                    NotificationManager.IMPORTANCE_DEFAULT
                )
                nm.createNotificationChannel(channel)
            }
            val notificationId = (System.currentTimeMillis() % 10000).toInt()
            val notification = NotificationCompat.Builder(appContext, channelId)
                .setSmallIcon(R.drawable.ic_launcher_foreground)
                .setContentTitle("Проверка сенсора Butler #$notificationId")
                .setContentText("Системное Android-уведомление для сквозной проверки NotificationListenerService")
                .setStyle(
                    NotificationCompat.BigTextStyle()
                        .bigText("Системное Android-уведомление #$notificationId. Если доступ к уведомлениям включён, служба перехватит его автоматически.")
                )
                .setPriority(NotificationCompat.PRIORITY_DEFAULT)
                .setAutoCancel(true)
                .build()

            nm.notify(notificationId, notification)
            _statusBannerMessage.value = "Отправлено системное уведомление Android #$notificationId"
        } catch (e: Exception) {
            _statusBannerMessage.value = "Не удалось отправить системное уведомление: ${e.localizedMessage}"
        }
    }

    // --- Pairing & Server Configuration ---

    fun updateServerAddressOnly(url: String, allowCleartext: Boolean) {
        val check = TransportPolicyValidator.validate(url, allowCleartext)
        credentialStore.updateServerUrl(url, allowCleartext)
        _statusBannerMessage.value = if (check.isValid) {
            "Адрес сервера обновлён: ${check.normalizedBaseUrl} (${check.classification.labelRu})"
        } else {
            check.warningOrErrorRu
        }
    }

    fun pairWithCodeOrQr(serverUrl: String, allowCleartext: Boolean, codeOrQr: String) {
        viewModelScope.launch {
            _isSyncingNow.value = true
            val res = repository.pairWithServerCodeOrQr(serverUrl, allowCleartext, codeOrQr)
            _isSyncingNow.value = false
            _statusBannerMessage.value = when (res) {
                is PairingResult.Paired -> "Устройство успешно сопряжено с «${res.serverName}»!"
                is PairingResult.Error -> "Ошибка сопряжения: ${res.messageRu}"
                else -> null
            }
        }
    }

    fun requestPairingCodeOnPhone(serverUrl: String, allowCleartext: Boolean) {
        viewModelScope.launch {
            _isSyncingNow.value = true
            val res = repository.initiatePhonePairingCode(serverUrl, allowCleartext)
            _isSyncingNow.value = false
            _statusBannerMessage.value = when (res) {
                is PairingResult.CodeRequested ->
                    "Код ${res.verificationCode} создан. Подтвердите его в Web UI сервера Butler."
                is PairingResult.Error -> "Ошибка запроса кода: ${res.messageRu}"
                else -> null
            }
        }
    }

    fun pollWebUiPairingApproval() {
        viewModelScope.launch {
            _isSyncingNow.value = true
            val res = repository.pollPhonePairingConfirmation()
            _isSyncingNow.value = false
            _statusBannerMessage.value = when (res) {
                is PairingResult.Paired -> "Подтверждение получено! Сопряжено с «${res.serverName}»."
                is PairingResult.PendingApproval -> res.messageRu
                is PairingResult.Error -> res.messageRu
                else -> null
            }
        }
    }

    fun disconnectDevice() {
        viewModelScope.launch {
            _isSyncingNow.value = true
            repository.disconnectDeviceAndWipe()
            _isSyncingNow.value = false
            _statusBannerMessage.value = "Устройство отключено, локальный токен и ключ Keystore уничтожены, локальная очередь очищена. Отзыв на сервере отправлен по возможности; при отсутствии связи отзовите устройство в Web UI."
        }
    }

    // --- Queue & Diagnostics Management ---

    fun decryptEventPayload(entity: QueuedNotificationEntity): NotificationPayloadDto? {
        return repository.decryptPayloadForUi(entity)
    }

    fun clearDeliveredQueueItems() {
        viewModelScope.launch {
            val count = repository.clearDeliveredEvents()
            _statusBannerMessage.value = "Удалено подтверждённых событий из очереди: $count"
        }
    }

    fun clearAllQueueItems() {
        viewModelScope.launch {
            repository.clearAllQueuedEvents()
            _statusBannerMessage.value = "Локальная очередь уведомлений полностью очищена"
        }
    }

    fun clearDiagnosticLogs() {
        viewModelScope.launch {
            repository.clearDiagnostics()
            _statusBannerMessage.value = "Журнал диагностики очищен"
        }
    }

    fun updateQueueLimits(maxCount: Int, maxMegabytes: Int, maxAgeDays: Int) {
        credentialStore.updateQueueLimits(
            maxCount = maxCount,
            maxBytes = maxMegabytes.toLong() * 1024L * 1024L,
            maxAgeDays = maxAgeDays
        )
        viewModelScope.launch {
            repository.enforceQueueRetentionAndCapacityLimits()
            _statusBannerMessage.value = "Лимиты очереди обновлены: до $maxCount событий / $maxMegabytes МБ / $maxAgeDays дн."
        }
    }

    companion object {
        fun provideFactory(app: ButlerApplication): ViewModelProvider.Factory =
            object : ViewModelProvider.Factory {
                @Suppress("UNCHECKED_CAST")
                override fun <T : ViewModel> create(modelClass: Class<T>): T {
                    return ButlerViewModel(
                        appContext = app.applicationContext,
                        repository = app.syncRepository,
                        credentialStore = app.credentialStore,
                        networkMonitor = app.networkMonitor
                    ) as T
                }
            }
    }
}
