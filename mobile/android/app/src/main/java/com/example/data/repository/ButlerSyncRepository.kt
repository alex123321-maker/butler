package com.example.data.repository

import com.example.data.local.AppFilterRuleEntity
import com.example.data.local.ButlerDao
import com.example.data.local.DiagnosticLogEntity
import com.example.data.local.QueuedNotificationEntity
import com.example.data.remote.ButlerMobileApiService
import com.example.data.remote.LoginOtpApiService
import com.example.data.remote.LoginOtpRequest
import com.example.domain.LoginOtpExtractor
import com.example.data.remote.LoopbackContractInspector
import com.example.data.remote.LoopbackHttpOutcome
import com.example.domain.NormalizationOutcome
import com.example.domain.NotificationEventNormalizer
import com.example.domain.PriorNotificationTrackingState
import com.example.domain.PrivacyRedactor
import com.example.domain.RawNotificationSnapshot
import com.example.model.AppFilterMode
import com.example.BuildConfig
import com.example.model.ButlerContractConstants
import com.example.model.DisconnectRequestDto
import com.example.model.MobileNotificationEventDto
import com.example.model.NotificationBatchRequestDto
import com.example.model.NotificationBatchResponseDto
import com.example.model.NotificationPayloadDto
import com.example.model.PairExchangeRequestDto
import com.example.model.PairPollRequestDto
import com.example.model.PairRequestInitDto
import com.example.model.RouteClassification
import com.example.security.DeviceCredentialStore
import com.example.security.KeystoreCryptoManager
import com.example.security.TransportPolicyValidator
import com.squareup.moshi.Moshi
import com.squareup.moshi.kotlin.reflect.KotlinJsonAdapterFactory
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import okhttp3.OkHttpClient
import retrofit2.Retrofit
import retrofit2.converter.moshi.MoshiConverterFactory
import java.io.IOException
import java.net.SocketTimeoutException
import java.net.UnknownHostException
import java.util.UUID
import java.util.concurrent.TimeUnit
import javax.net.ssl.SSLException
import kotlin.math.min
import kotlin.math.pow
import kotlin.random.Random

sealed class SyncBatchResult {
    data class Success(
        val acceptedCount: Int,
        val duplicateCount: Int,
        val conflictCount: Int,
        val latencyMs: Long
    ) : SyncBatchResult()

    data class Idle(val reasonRu: String) : SyncBatchResult()
    data class Failure(val category: String, val messageRu: String, val retryScheduled: Boolean) : SyncBatchResult()
}

sealed class PairingResult {
    data class Paired(val serverName: String, val serverUrl: String) : PairingResult()
    data class CodeRequested(val sessionId: String, val verificationCode: String, val expiresInSec: Int) : PairingResult()
    data class PendingApproval(val messageRu: String) : PairingResult()
    data class Error(val messageRu: String) : PairingResult()
}

class ButlerSyncRepository(
    private val dao: ButlerDao,
    private val credentialStore: DeviceCredentialStore,
    private val cryptoManager: KeystoreCryptoManager,
    private val normalizer: NotificationEventNormalizer = NotificationEventNormalizer(),
    val loopbackInspector: LoopbackContractInspector = LoopbackContractInspector(),
    private val apiServiceOverride: ButlerMobileApiService? = null,
    private val defaultSmsPackage: () -> String? = { null },
    private val notificationAccessGranted: () -> Boolean = { true }
) {
    private val syncMutex = Mutex()
    private val captureMutex = Mutex()
    private val otpScope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val otpSlots = java.util.concurrent.Semaphore(4)
    private val moshi: Moshi = Moshi.Builder()
        .add(KotlinJsonAdapterFactory())
        .build()
    private val payloadAdapter = moshi.adapter(NotificationPayloadDto::class.java)

    val recentQueueEvents: Flow<List<QueuedNotificationEntity>> = dao.observeRecentQueueEvents()
    val pendingCount: Flow<Int> = dao.observePendingCount()
    val deliveredCount: Flow<Int> = dao.observeDeliveredCount()
    val errorCount: Flow<Int> = dao.observeErrorCount()
    val totalQueueBytes: Flow<Long> = dao.observeTotalQueueBytes()
    val appFilterRules: Flow<List<AppFilterRuleEntity>> = dao.observeAppFilterRules()
    val diagnosticLogs: Flow<List<DiagnosticLogEntity>> = dao.observeDiagnosticLogs()

    suspend fun ensureDefaultAppRulesSeeded() = syncMutex.withLock {
        val defaults = listOf(
            AppFilterRuleEntity(
                packageName = "org.telegram.messenger",
                appLabel = "Telegram",
                categoryLabel = "Мессенджеры",
                isSensitiveSource = false,
                isExcludedInAllMode = false,
                isIncludedInAllowlistMode = true
            ),
            AppFilterRuleEntity(
                packageName = "com.whatsapp",
                appLabel = "WhatsApp",
                categoryLabel = "Мессенджеры",
                isSensitiveSource = false,
                isExcludedInAllMode = false,
                isIncludedInAllowlistMode = true
            ),
            AppFilterRuleEntity(
                packageName = "com.google.android.gm",
                appLabel = "Gmail",
                categoryLabel = "Почта и работа",
                isSensitiveSource = false,
                isExcludedInAllMode = false,
                isIncludedInAllowlistMode = true
            ),
            AppFilterRuleEntity(
                packageName = "com.google.android.calendar",
                appLabel = "Google Календарь",
                categoryLabel = "Планирование",
                isSensitiveSource = false,
                isExcludedInAllMode = false,
                isIncludedInAllowlistMode = true
            ),
            AppFilterRuleEntity(
                packageName = "com.github.android",
                appLabel = "GitHub",
                categoryLabel = "Разработка",
                isSensitiveSource = false,
                isExcludedInAllMode = false,
                isIncludedInAllowlistMode = true
            ),
            AppFilterRuleEntity(
                packageName = "ru.sberbankmobile",
                appLabel = "СберБанк Онлайн",
                categoryLabel = "Банки и финансы",
                isSensitiveSource = true,
                isExcludedInAllMode = false,
                isIncludedInAllowlistMode = false
            ),
            AppFilterRuleEntity(
                packageName = "com.idamob.tinkoff.android",
                appLabel = "Т-Банк (Тинькофф)",
                categoryLabel = "Банки и финансы",
                isSensitiveSource = true,
                isExcludedInAllMode = false,
                isIncludedInAllowlistMode = false
            ),
            AppFilterRuleEntity(
                packageName = "com.google.android.apps.authenticator2",
                appLabel = "Google Authenticator",
                categoryLabel = "Коды 2FA / Безопасность",
                isSensitiveSource = true,
                isExcludedInAllMode = true,
                isIncludedInAllowlistMode = false
            ),
            AppFilterRuleEntity(
                packageName = "com.android.systemui",
                appLabel = "Система Android (System UI)",
                categoryLabel = "Системные",
                isSensitiveSource = false,
                isExcludedInAllMode = false,
                isIncludedInAllowlistMode = false
            )
        )
        dao.insertDefaultAppFilterRules(defaults)
        // Auto-repair any previously saved rules that got a generic "Android" label from ".android" suffix
        val existingRules = dao.getAllAppFilterRules()
        for (rule in existingRules) {
            val betterLabel = inferAppLabel(rule.packageName)
            if ((rule.appLabel.equals("Android", ignoreCase = true) || rule.appLabel.isBlank()) &&
                !betterLabel.equals(rule.appLabel, ignoreCase = false)
            ) {
                dao.upsertAppFilterRule(rule.copy(appLabel = betterLabel))
            }
        }
        val recovered = dao.recoverStaleInFlightEvents()
        if (recovered > 0) {
            logDiagnostic(
                level = "INFO",
                stage = "QUEUE",
                code = "IN_FLIGHT_RECOVERED",
                summaryRu = "Восстановлено $recovered прерванных событий после перезапуска процесса"
            )
        }
    }

    suspend fun shouldCapturePackage(packageName: String, appLabelHint: String? = null): Boolean {
        val config = credentialStore.state.value
        var rule = dao.getAppFilterRule(packageName)
        val resolvedLabel = appLabelHint?.takeIf {
            it.isNotBlank() && !it.equals("Android", ignoreCase = true)
        } ?: inferAppLabel(packageName)

        if (rule == null) {
            val isSensitive = PrivacyRedactor.isSensitivePackage(packageName)
            rule = AppFilterRuleEntity(
                packageName = packageName,
                appLabel = resolvedLabel,
                categoryLabel = if (isSensitive) "Банки / Чувствительные" else "Приложения",
                isSensitiveSource = isSensitive,
                isExcludedInAllMode = false,
                isIncludedInAllowlistMode = false,
                lastSeenNotificationEpochMs = System.currentTimeMillis(),
                totalObservedCount = 0
            )
            dao.upsertAppFilterRule(rule)
        } else if (rule.appLabel.equals("Android", ignoreCase = true) && resolvedLabel != "Android") {
            rule = rule.copy(appLabel = resolvedLabel)
            dao.upsertAppFilterRule(rule)
        }

        return when (config.appFilterMode) {
            AppFilterMode.ALL_EXCEPT_EXCLUDED -> !rule.isExcludedInAllMode
            AppFilterMode.ONLY_INCLUDED -> rule.isIncludedInAllowlistMode
        }
    }

    suspend fun setPackageEnabledForCurrentMode(packageName: String, enabled: Boolean) {
        val config = credentialStore.state.value
        val existing = dao.getAppFilterRule(packageName) ?: AppFilterRuleEntity(
            packageName = packageName,
            appLabel = inferAppLabel(packageName),
            categoryLabel = if (PrivacyRedactor.isSensitivePackage(packageName)) "Чувствительные" else "Приложения",
            isSensitiveSource = PrivacyRedactor.isSensitivePackage(packageName),
            isExcludedInAllMode = false,
            isIncludedInAllowlistMode = true
        )
        val updated = when (config.appFilterMode) {
            AppFilterMode.ALL_EXCEPT_EXCLUDED -> existing.copy(isExcludedInAllMode = !enabled)
            AppFilterMode.ONLY_INCLUDED -> existing.copy(isIncludedInAllowlistMode = enabled)
        }
        dao.upsertAppFilterRule(updated)
    }

    suspend fun addCustomPackageFilterRule(packageName: String, appLabel: String) {
        val cleanPkg = packageName.trim()
        if (cleanPkg.isEmpty()) return
        val isSensitive = PrivacyRedactor.isSensitivePackage(cleanPkg)
        val entity = AppFilterRuleEntity(
            packageName = cleanPkg,
            appLabel = appLabel.trim().ifEmpty { inferAppLabel(cleanPkg) },
            categoryLabel = if (isSensitive) "Банки / Чувствительные" else "Пользовательские",
            isSensitiveSource = isSensitive,
            isExcludedInAllMode = false,
            isIncludedInAllowlistMode = true
        )
        dao.upsertAppFilterRule(entity)
    }

    /**
     * Normalizes, encrypts, and persists a posted or updated notification into the local Room queue.
     * Returns the created [QueuedNotificationEntity] if emitted, or null if filtered/deduplicated.
     */
    suspend fun ingestRawNotification(
        snapshot: RawNotificationSnapshot,
        isInitialSnapshot: Boolean = false,
        bypassPauseForManualTest: Boolean = false,
        deterministicEventId: String? = null
    ): QueuedNotificationEntity? = captureMutex.withLock {
        val config = credentialStore.state.value
        val allowed = shouldCapturePackage(snapshot.packageName, snapshot.appLabelHint)
        if (!bypassPauseForManualTest) {
            if (!config.isPaired || !notificationAccessGranted() || config.isSyncPaused || config.isTokenRevoked || !allowed) {
                return null
            }
        }

        val latestForKey = dao.getLatestActiveEventForKey(snapshot.notificationKey)
        val smsPackage = snapshot.packageName
        if (!isInitialSnapshot && !snapshot.isGroupSummary && snapshot.messages.size <= 1 && !bypassPauseForManualTest && config.allowLoginOtpRelay && config.isContentTransmissionEnabled &&
            smsPackage == defaultSmsPackage() && otpSlots.tryAcquire()) {
            val message = snapshot.messages.singleOrNull()
            val candidate = LoginOtpExtractor.extract(snapshot.notificationKey, message?.sender?.takeIf { it.isNotBlank() } ?: snapshot.title,
                listOfNotNull(snapshot.text, snapshot.bigText, message?.text).joinToString(" "), snapshot.postedAtEpochMs,
                System.currentTimeMillis(), snapshot.platformRedactedOtp || snapshot.visibility == com.example.model.NotificationVisibilityLevel.SECRET)
            if (candidate == null) otpSlots.release() else otpScope.launch {
                try {
                    for (wait in listOf(0L, 3_000L, 5_000L, 10_000L, 10_000L, 10_000L)) {
                        delay(wait)
                        val live = credentialStore.state.value
                        if (!live.allowLoginOtpRelay || !live.isContentTransmissionEnabled || !live.isPaired || live.isTokenRevoked || live.isSyncPaused ||
                            !notificationAccessGranted() || defaultSmsPackage() != smsPackage) break
                        val route = TransportPolicyValidator.validate(live.serverUrl, live.allowCleartextLocalHttp)
                        if (!route.isValid || route.classification == RouteClassification.LOOPBACK_SANDBOX) break
                        val deviceToken = credentialStore.getDecryptedDeviceToken() ?: break
                        val accepted = withTimeoutOrNull(3_000) {
                            try {
                                val api = buildRetrofit(route.normalizedBaseUrl).create(LoginOtpApiService::class.java)
                                val response = api.submit("Bearer $deviceToken", live.deviceId,
                                    LoginOtpRequest(candidate.eventId, smsPackage, candidate.sender, candidate.code, candidate.postedAt))
                                response.isSuccessful && response.body()?.accepted == true
                            } catch (_: Exception) { false }
                        }
                        if (accepted == true) break
                    }
                } finally { otpSlots.release() }
            }
        }
        val priorState = latestForKey?.takeIf { it.changeType != "REMOVED" }?.let {
            PriorNotificationTrackingState(
                lastDeviceEventId = it.deviceEventId,
                lastContentHash = it.contentHash
            )
        }

        val outcome = normalizer.normalizePostedOrUpdated(
            snapshot = snapshot,
            isInitialSnapshot = isInitialSnapshot,
            priorState = priorState,
            isContentTransmissionEnabled = config.isContentTransmissionEnabled,
            maskOtpAndBankCodes = config.maskOtpAndBankCodes || (config.allowLoginOtpRelay && snapshot.packageName == defaultSmsPackage()),
            deterministicEventId = deterministicEventId
        )

        return when (outcome) {
            is NormalizationOutcome.DeduplicatedUnchanged -> {
                null
            }
            is NormalizationOutcome.Emitted -> {
                val entity = persistNormalizedEvent(outcome.event)
                updateAppRuleStats(snapshot.packageName, snapshot.observedAtEpochMs)
                entity
            }
        }
    }

    suspend fun ingestNotificationRemoved(
        notificationKey: String,
        packageName: String,
        channelId: String?,
        postedAtEpochMs: Long,
        observedAtEpochMs: Long = System.currentTimeMillis(),
        deterministicEventId: String? = null
    ): QueuedNotificationEntity? = captureMutex.withLock {
        val config = credentialStore.state.value
        if (!config.isPaired || !notificationAccessGranted() || config.isSyncPaused || config.isTokenRevoked) return null
        if (!shouldCapturePackage(packageName)) return null

        val latestForKey = dao.getLatestActiveEventForKey(notificationKey)
        if (latestForKey?.changeType == "REMOVED") return null
        val priorState = latestForKey?.let {
            PriorNotificationTrackingState(
                lastDeviceEventId = it.deviceEventId,
                lastContentHash = it.contentHash
            )
        }

        val event = normalizer.normalizeRemoved(
            notificationKey = notificationKey,
            packageName = packageName,
            channelId = channelId,
            postedAtEpochMs = postedAtEpochMs,
            observedAtEpochMs = observedAtEpochMs,
            priorState = priorState,
            deterministicEventId = deterministicEventId
        )
        return persistNormalizedEvent(event)
    }

    private suspend fun persistNormalizedEvent(event: MobileNotificationEventDto): QueuedNotificationEntity {
        enforceQueueRetentionAndCapacityLimits()

        val encryptedBlob: String? = event.payload?.let { payload ->
            val json = payloadAdapter.toJson(payload)
            cryptoManager.encryptText(json)
        }
        val sizeBytes = (encryptedBlob?.toByteArray(Charsets.UTF_8)?.size ?: 0) + 2048

        val entity = QueuedNotificationEntity(
            deviceEventId = event.deviceEventId,
            schemaVersion = event.schemaVersion,
            notificationKey = event.notificationKey,
            previousDeviceEventId = event.previousDeviceEventId,
            packageName = event.packageName,
            channelId = event.channelId,
            changeType = event.changeType,
            isInitialSnapshot = event.isInitialSnapshot,
            postedAtEpochMs = event.postedAtEpochMs,
            observedAtEpochMs = event.observedAtEpochMs,
            category = event.category,
            visibility = event.visibility,
            isGroupSummary = event.isGroupSummary,
            groupKey = event.groupKey,
            isOngoing = event.isOngoing,
            isClearable = event.isClearable,
            contentRedactedByUserRule = event.contentRedactedByUserRule,
            contentOmittedByMode = event.contentOmittedByMode,
            systemOtpRedacted = event.systemOtpRedacted,
            isTruncated = event.isTruncated,
            contentHash = event.contentHash,
            encryptedPayloadBlob = encryptedBlob,
            payloadSizeBytes = sizeBytes,
            deliveryState = "PENDING",
            attemptCount = 0,
            nextAttemptAtEpochMs = 0L
        )
        dao.insertQueuedEvent(entity)
        enforceQueueRetentionAndCapacityLimits()
        return entity
    }

    /**
     * Enforces local queue age, count, and byte volume limits.
     * Never silently drops unacknowledged events: if overflow occurs after purging delivered items,
     * increments `droppedOverflowCount` in [DeviceCredentialStore] and logs a zero-PII diagnostic warning.
     */
    suspend fun enforceQueueRetentionAndCapacityLimits(nowEpochMs: Long = System.currentTimeMillis()) {
        val config = credentialStore.state.value
        val ageCutoffMs = nowEpochMs - config.maxQueueAgeDays * 24L * 60L * 60L * 1000L
        val expired = dao.countUndeliveredBefore(ageCutoffMs)
        if (expired > 0) credentialStore.incrementDroppedOverflowCount(expired)
        dao.deleteEventsOlderThan(ageCutoffMs)

        var totalCount = dao.getTotalEventCount()
        var totalBytes = dao.getTotalQueueBytes()

        if (totalCount > config.maxQueueCount || totalBytes > config.maxQueueBytes) {
            // Step 1: Purge already delivered events first
            dao.purgeOldestDeliveredEvents(limit = 100)
            totalCount = dao.getTotalEventCount()
            totalBytes = dao.getTotalQueueBytes()
        }

        while (totalCount > config.maxQueueCount || totalBytes > config.maxQueueBytes) {
            val overflowToEvict = (totalCount - config.maxQueueCount).coerceAtLeast(1)
            val evicted = dao.evictOldestEventsOnOverflow(overflowToEvict)
            if (evicted > 0) {
                val totalDropped = credentialStore.incrementDroppedOverflowCount(evicted)
                logDiagnostic(
                    level = "WARN",
                    stage = "QUEUE",
                    code = "QUEUE_OVERFLOW_DROP",
                    summaryRu = "Переполнение локальной очереди: вытеснено $evicted старых событий (всего потерь: $totalDropped)"
                )
            }
            if (evicted == 0) break
            totalCount = dao.getTotalEventCount()
            totalBytes = dao.getTotalQueueBytes()
        }
    }

    fun decryptPayloadForUi(entity: QueuedNotificationEntity): NotificationPayloadDto? {
        val blob = entity.encryptedPayloadBlob ?: return null
        return try {
            val json = cryptoManager.decryptText(blob)
            payloadAdapter.fromJson(json)
        } catch (_: Exception) {
            null
        }
    }

    /**
     * Sends ready batches to the Butler server with concurrency protection, per-ID ACK verification,
     * conflict detection, and exponential backoff with random jitter.
     */
    suspend fun syncPendingQueueBatch(
        ignoreBackoffTimers: Boolean = false,
        maxBatchSize: Int = 5
    ): SyncBatchResult = syncMutex.withLock {
        val config = credentialStore.state.value
        if (config.isTokenRevoked) {
            return SyncBatchResult.Failure(
                category = "AUTH_REVOKED",
                messageRu = "Токен устройства отозван. Требуется повторное сопряжение.",
                retryScheduled = false
            )
        }
        if (!config.isPaired) {
            return SyncBatchResult.Idle("Устройство ещё не сопряжено с сервером Butler")
        }
        if (config.isSyncPaused || !notificationAccessGranted()) {
            return SyncBatchResult.Idle("Синхронизация поставлена на паузу владельцем")
        }

        val transportCheck = TransportPolicyValidator.validate(
            rawUrl = config.serverUrl,
            allowCleartextLocalHttp = config.allowCleartextLocalHttp
        )
        if (!transportCheck.isValid) {
            val msg = transportCheck.warningOrErrorRu ?: "Недопустимый адрес сервера"
            credentialStore.recordSyncError("TRANSPORT_POLICY", msg)
            logDiagnostic("ERROR", "SECURITY", "INSECURE_TRANSPORT_BLOCKED", msg)
            return SyncBatchResult.Failure("TRANSPORT_POLICY", msg, retryScheduled = false)
        }

        val queryTimeMs = if (ignoreBackoffTimers) Long.MAX_VALUE else System.currentTimeMillis()
        var readyEntities = dao.getReadyBatchForSync(nowEpochMs = queryTimeMs, limit = maxBatchSize.coerceIn(1, 50))
        if (readyEntities.isEmpty()) {
            return SyncBatchResult.Idle("Очередь отправки пуста или ожидает окна повтора")
        }
        if (!config.isContentTransmissionEnabled && readyEntities.any { it.encryptedPayloadBlob != null }) {
            return SyncBatchResult.Idle("Очистите старую очередь с текстом перед передачей только метаданных")
        }

        val batchIds = readyEntities.map { it.deviceEventId }

        var wireEvents = readyEntities.map { entity ->
            val decryptedPayload = decryptPayloadForUi(entity)
            if (entity.encryptedPayloadBlob != null && decryptedPayload == null) {
                dao.scheduleRetryForBatch(batchIds, System.currentTimeMillis() + 60_000, "LOCAL_CRYPTO_ERROR", "Локальная ошибка шифрования")
                return SyncBatchResult.Failure("LOCAL_CRYPTO_ERROR", "Сохранённое событие не расшифровано; пакет не отправлен", false)
            }
            MobileNotificationEventDto(
                schemaVersion = entity.schemaVersion,
                deviceEventId = entity.deviceEventId,
                notificationKey = entity.notificationKey,
                previousDeviceEventId = entity.previousDeviceEventId,
                packageName = entity.packageName,
                channelId = entity.channelId,
                changeType = entity.changeType,
                isInitialSnapshot = entity.isInitialSnapshot,
                postedAtEpochMs = entity.postedAtEpochMs,
                observedAtEpochMs = entity.observedAtEpochMs,
                category = entity.category,
                visibility = entity.visibility,
                isGroupSummary = entity.isGroupSummary,
                groupKey = entity.groupKey,
                isOngoing = entity.isOngoing,
                isClearable = entity.isClearable,
                contentRedactedByUserRule = entity.contentRedactedByUserRule,
                contentOmittedByMode = entity.contentOmittedByMode,
                systemOtpRedacted = entity.systemOtpRedacted,
                isTruncated = entity.isTruncated,
                contentHash = entity.contentHash,
                payload = decryptedPayload
            )
        }

        val eventAdapter = moshi.adapter(MobileNotificationEventDto::class.java)
        var totalBytes = 0
        wireEvents = wireEvents.takeWhile { event ->
            totalBytes += eventAdapter.toJson(event).toByteArray(Charsets.UTF_8).size
            totalBytes < 240 * 1024
        }
        readyEntities = readyEntities.take(wireEvents.size)
        if (wireEvents.isEmpty()) return SyncBatchResult.Failure("EVENT_SIZE_LIMIT", "Событие превышает лимит пакета", false)
        dao.markBatchInFlight(readyEntities.map { it.deviceEventId })
        val batchRequest = NotificationBatchRequestDto(
            schemaVersion = ButlerContractConstants.CURRENT_SCHEMA_VERSION,
            batchId = UUID.randomUUID().toString(),
            sentAtEpochMs = System.currentTimeMillis(),
            events = wireEvents
        )

        val startMs = System.currentTimeMillis()
        val token = credentialStore.getDecryptedDeviceToken()
        if (token.isNullOrBlank()) {
            credentialStore.markTokenRevoked("Отсутствует локальный ключ устройства в Keystore")
            return SyncBatchResult.Failure("AUTH_REVOKED", "Токен недоступен", retryScheduled = false)
        }

        return try {
            if (transportCheck.classification == RouteClassification.LOOPBACK_SANDBOX && apiServiceOverride == null) {
                val outcome = loopbackInspector.processBatch(batchRequest, config.loopbackSimulationMode)
                val latency = (System.currentTimeMillis() - startMs).coerceAtLeast(4L)
                handleLoopbackBatchOutcome(readyEntities, outcome, latency)
            } else {
                val api = apiServiceOverride ?: buildRetrofitApi(transportCheck.normalizedBaseUrl)
                val response = api.postNotificationBatch(
                    bearerToken = "Bearer $token",
                    deviceId = config.deviceId,
                    schemaVersion = ButlerContractConstants.CURRENT_SCHEMA_VERSION,
                    batch = batchRequest
                )
                val latency = (System.currentTimeMillis() - startMs).coerceAtLeast(1L)
                handleHttpResponseBatchOutcome(readyEntities, response.code(), response.body(), response.headers()["Retry-After"], latency)
            }
        } catch (e: Exception) {
            val latency = (System.currentTimeMillis() - startMs).coerceAtLeast(1L)
            handleNetworkExceptionOnBatch(readyEntities, e, latency)
        }
    }

    private suspend fun handleLoopbackBatchOutcome(
        entities: List<QueuedNotificationEntity>,
        outcome: LoopbackHttpOutcome<NotificationBatchResponseDto>,
        latencyMs: Long
    ): SyncBatchResult {
        return when (outcome) {
            is LoopbackHttpOutcome.Success -> applyServerBatchAck(
                entities = entities,
                httpCode = outcome.httpCode,
                body = outcome.body,
                latencyMs = latencyMs
            )
            is LoopbackHttpOutcome.HttpError -> applyServerHttpError(
                entities = entities,
                httpCode = outcome.httpCode,
                errorCode = outcome.errorCode,
                errorMessage = outcome.message,
                retryAfterSec = outcome.retryAfterSec,
                latencyMs = latencyMs
            )
            is LoopbackHttpOutcome.NetworkFailure -> handleNetworkExceptionOnBatch(
                entities = entities,
                exception = outcome.exception,
                latencyMs = latencyMs
            )
        }
    }

    private suspend fun handleHttpResponseBatchOutcome(
        entities: List<QueuedNotificationEntity>,
        httpCode: Int,
        body: NotificationBatchResponseDto?,
        retryAfterHeader: String?,
        latencyMs: Long
    ): SyncBatchResult {
        if (httpCode in 200..299 && body != null) {
            return applyServerBatchAck(entities, httpCode, body, latencyMs)
        }
        val retryAfterSec = retryAfterHeader?.toIntOrNull()
        val errorCode = when (httpCode) {
            400 -> "SCHEMA_VALIDATION_ERROR"
            401, 403 -> "DEVICE_TOKEN_REVOKED"
            409 -> "BATCH_ID_HASH_CONFLICT"
            429 -> "RATE_LIMITED"
            else -> "HTTP_$httpCode"
        }
        val messageRu = when (httpCode) {
            400 -> "Сервер отклонил формат пакета (400 Bad Request)"
            401, 403 -> "Токен устройства недействителен или отозван на сервере ($httpCode)"
            409 -> "Конфликт идентификатора и хеша события на сервере (409 Conflict)"
            429 -> "Превышена частота отправки пакетов (429 Too Many Requests)"
            else -> "Ошибка сервера Butler (HTTP $httpCode)"
        }
        return applyServerHttpError(entities, httpCode, errorCode, messageRu, retryAfterSec, latencyMs)
    }

    private suspend fun applyServerBatchAck(
        entities: List<QueuedNotificationEntity>,
        httpCode: Int,
        body: NotificationBatchResponseDto,
        latencyMs: Long
    ): SyncBatchResult {
        val now = System.currentTimeMillis()
        val sentIds = entities.map { it.deviceEventId }.toSet()
        val responseIds = body.acceptedIds + body.duplicateIds + body.conflictIds
        val rejectedIds = body.rejected.map { it.deviceEventId }
        if (responseIds.any { it !in sentIds } || responseIds.size != responseIds.distinct().size) {
            return handleNetworkExceptionOnBatch(entities, IOException("Invalid ACK"), latencyMs)
        }
        if (rejectedIds.any { it !in sentIds || it in body.acceptedIds || it in body.duplicateIds } || rejectedIds.size != rejectedIds.distinct().size) {
            return handleNetworkExceptionOnBatch(entities, IOException("Invalid ACK"), latencyMs)
        }
        val confirmedIds = (body.acceptedIds + body.duplicateIds).distinct()
        if (confirmedIds.isNotEmpty()) {
            dao.markEventsDelivered(confirmedIds, now)
        }

        val conflictSet = body.conflictIds.toMutableSet()
        body.rejected.forEach { rej ->
            conflictSet.add(rej.deviceEventId)
            dao.markEventConflictError(
                deviceEventId = rej.deviceEventId,
                errorCode = if (rej.errorCode in setOf("SOURCE_DELETED", "ID_CONTENT_HASH_CONFLICT", "SCHEMA_VALIDATION_ERROR")) rej.errorCode else "EVENT_REJECTED",
                reason = "Сервер отклонил событие"
            )
        }
        for (conflictId in body.conflictIds) {
            if (body.rejected.none { it.deviceEventId == conflictId }) {
                dao.markEventConflictError(
                    deviceEventId = conflictId,
                    errorCode = "ID_CONTENT_HASH_CONFLICT",
                    reason = "Сервер сообщил о конфликте хеша для существующего device_event_id"
                )
            }
        }

        // Any event not explicitly in accepted, duplicate, or conflict remains unconfirmed and is scheduled for retry
        val accountedIds = (confirmedIds + conflictSet).toSet()
        val unconfirmedEntities = entities.filter { it.deviceEventId !in accountedIds }
        if (unconfirmedEntities.isNotEmpty()) {
            val maxAttempt = unconfirmedEntities.maxOf { it.attemptCount }
            val nextRetryMs = now + computeExponentialBackoffMs(maxAttempt)
            dao.scheduleRetryForBatch(
                ids = unconfirmedEntities.map { it.deviceEventId },
                nextAttemptAtMs = nextRetryMs,
                errorCode = "PARTIAL_ACK_UNCONFIRMED",
                reason = "Событие не вошло в список подтверждённых сервером (повтор)"
            )
        }

        if (conflictSet.isNotEmpty()) {
            val warnMsg = "Пакет обработан частично: подтверждено ${confirmedIds.size}, конфликтов ID/хеша: ${conflictSet.size}"
            credentialStore.recordSyncError("CONFLICT_409", warnMsg)
            logDiagnostic("WARN", "NETWORK_SYNC", "BATCH_CONFLICT_DETECTED", warnMsg, httpCode, entities.size, latencyMs)
        } else {
            credentialStore.recordSyncSuccess(now)
            logDiagnostic(
                level = "INFO",
                stage = "NETWORK_SYNC",
                code = "BATCH_ACK_OK",
                summaryRu = "Пакет подтверждён сервером: принято ${body.acceptedIds.size}, дублей ${body.duplicateIds.size}",
                httpStatus = httpCode,
                batchSize = entities.size,
                latencyMs = latencyMs
            )
        }

        return SyncBatchResult.Success(
            acceptedCount = body.acceptedIds.size,
            duplicateCount = body.duplicateIds.size,
            conflictCount = conflictSet.size,
            latencyMs = latencyMs
        )
    }

    private suspend fun applyServerHttpError(
        entities: List<QueuedNotificationEntity>,
        httpCode: Int,
        errorCode: String,
        errorMessage: String,
        retryAfterSec: Int?,
        latencyMs: Long
    ): SyncBatchResult {
        val ids = entities.map { it.deviceEventId }
        val now = System.currentTimeMillis()

        when (httpCode) {
            401, 403 -> {
                // Keep events safely in PENDING state, revoke token, and stop sync
                dao.scheduleRetryForBatch(ids, now, errorCode, errorMessage)
                credentialStore.markTokenRevoked(errorMessage)
                logDiagnostic("ERROR", "SECURITY", errorCode, errorMessage, httpCode, entities.size, latencyMs)
                return SyncBatchResult.Failure("AUTH_REVOKED", errorMessage, retryScheduled = false)
            }
            409 -> {
                // Never overwrite or infinitely retry conflicting IDs
                ids.forEach { id ->
                    dao.markEventConflictError(id, errorCode, errorMessage)
                }
                credentialStore.recordSyncError("CONFLICT_409", errorMessage)
                logDiagnostic("ERROR", "NETWORK_SYNC", errorCode, errorMessage, httpCode, entities.size, latencyMs)
                return SyncBatchResult.Failure("CONFLICT_409", errorMessage, retryScheduled = false)
            }
            else -> {
                val maxAttempt = entities.maxOfOrNull { it.attemptCount } ?: 0
                val delayMs = if (retryAfterSec != null && retryAfterSec > 0) {
                    retryAfterSec * 1000L + Random.nextLong(200L, 900L)
                } else {
                    computeExponentialBackoffMs(maxAttempt)
                }
                dao.scheduleRetryForBatch(ids, now + delayMs, errorCode, errorMessage)
                credentialStore.recordSyncError(errorCode, "$errorMessage (повтор через ${delayMs / 1000} с)")
                logDiagnostic("WARN", "NETWORK_SYNC", errorCode, errorMessage, httpCode, entities.size, latencyMs)
                return SyncBatchResult.Failure(errorCode, errorMessage, retryScheduled = true)
            }
        }
    }

    private suspend fun handleNetworkExceptionOnBatch(
        entities: List<QueuedNotificationEntity>,
        exception: Exception,
        latencyMs: Long
    ): SyncBatchResult {
        val (category, messageRu) = classifyNetworkException(exception)
        val ids = entities.map { it.deviceEventId }
        val maxAttempt = entities.maxOfOrNull { it.attemptCount } ?: 0
        val backoffMs = computeExponentialBackoffMs(maxAttempt)
        val now = System.currentTimeMillis()

        dao.scheduleRetryForBatch(
            ids = ids,
            nextAttemptAtMs = now + backoffMs,
            errorCode = category,
            reason = messageRu
        )
        credentialStore.recordSyncError(category, "$messageRu (повтор через ${backoffMs / 1000} с)")
        logDiagnostic("WARN", "NETWORK_SYNC", category, messageRu, null, entities.size, latencyMs)
        return SyncBatchResult.Failure(category, messageRu, retryScheduled = true)
    }

    fun computeExponentialBackoffMs(attemptCount: Int): Long {
        val cappedAttempt = attemptCount.coerceIn(0, 8)
        val baseDelayMs = 2_000.0 * 2.0.pow(cappedAttempt.toDouble())
        val boundedMs = min(300_000.0, baseDelayMs).toLong()
        val jitterMs = Random.nextLong(150L, 1_100L)
        return boundedMs + jitterMs
    }

    // --- Pairing & Connection Flows ---

    suspend fun pairWithServerCodeOrQr(
        rawServerUrl: String,
        allowCleartextHttp: Boolean,
        codeOrQrPayload: String
    ): PairingResult {
        val parsed = parsePairingInput(rawServerUrl, codeOrQrPayload)
        val validation = TransportPolicyValidator.validate(parsed.serverUrl, allowCleartextHttp)
        if (!validation.isValid) {
            return PairingResult.Error(validation.warningOrErrorRu ?: "Некорректный адрес сервера")
        }

        val config = credentialStore.state.value
        val req = PairExchangeRequestDto(
            pairingCode = parsed.code,
            deviceId = config.deviceId,
            deviceName = config.deviceName,
            androidSdk = android.os.Build.VERSION.SDK_INT,
            appVersion = BuildConfig.VERSION_NAME
        )

        return try {
            if (validation.classification == RouteClassification.LOOPBACK_SANDBOX && apiServiceOverride == null) {
                when (val res = loopbackInspector.exchangeCode(req)) {
                    is LoopbackHttpOutcome.Success -> {
                        credentialStore.updateServerUrl(validation.normalizedBaseUrl, allowCleartextHttp)
                        credentialStore.savePairingSuccess(
                            serverUrl = validation.normalizedBaseUrl,
                            serverName = res.body.serverName,
                            deviceToken = res.body.deviceToken
                        )
                        logDiagnostic("INFO", "PAIRING", "PAIRED_BY_CODE", "Успешное сопряжение с ${res.body.serverName}")
                        PairingResult.Paired(res.body.serverName, validation.normalizedBaseUrl)
                    }
                    is LoopbackHttpOutcome.HttpError -> PairingResult.Error(res.message)
                    is LoopbackHttpOutcome.NetworkFailure -> PairingResult.Error(res.exception.message ?: "Ошибка сети")
                }
            } else {
                val api = apiServiceOverride ?: buildRetrofitApi(validation.normalizedBaseUrl)
                val response = api.exchangePairingCode(req)
                val body = response.body()
                if (response.isSuccessful && body != null) {
                    credentialStore.updateServerUrl(validation.normalizedBaseUrl, allowCleartextHttp)
                    credentialStore.savePairingSuccess(
                        serverUrl = validation.normalizedBaseUrl,
                        serverName = body.serverName,
                        deviceToken = body.deviceToken
                    )
                    logDiagnostic("INFO", "PAIRING", "PAIRED_BY_CODE", "Успешное сопряжение с ${body.serverName}", response.code())
                    PairingResult.Paired(body.serverName, validation.normalizedBaseUrl)
                } else {
                    PairingResult.Error("Сервер отклонил код сопряжения (HTTP ${response.code()})")
                }
            }
        } catch (e: Exception) {
            val (_, msg) = classifyNetworkException(e)
            PairingResult.Error(msg)
        }
    }

    suspend fun initiatePhonePairingCode(
        rawServerUrl: String,
        allowCleartextHttp: Boolean
    ): PairingResult {
        val validation = TransportPolicyValidator.validate(rawServerUrl, allowCleartextHttp)
        if (!validation.isValid) {
            return PairingResult.Error(validation.warningOrErrorRu ?: "Некорректный адрес сервера")
        }
        credentialStore.updateServerUrl(validation.normalizedBaseUrl, allowCleartextHttp)
        val config = credentialStore.state.value
        val req = PairRequestInitDto(
            deviceId = config.deviceId,
            deviceName = config.deviceName,
            androidSdk = android.os.Build.VERSION.SDK_INT,
            appVersion = BuildConfig.VERSION_NAME
        )

        return try {
            if (validation.classification == RouteClassification.LOOPBACK_SANDBOX && apiServiceOverride == null) {
                when (val res = loopbackInspector.requestPhonePairing(req)) {
                    is LoopbackHttpOutcome.Success -> {
                        credentialStore.savePendingPhonePairingSession(
                            sessionId = res.body.pairingSessionId,
                            verificationCode = res.body.userVerificationCode
                        )
                        logDiagnostic("INFO", "PAIRING", "PAIR_CODE_REQUESTED", "Запрошен проверочный код сопряжения на телефоне")
                        PairingResult.CodeRequested(
                            sessionId = res.body.pairingSessionId,
                            verificationCode = res.body.userVerificationCode,
                            expiresInSec = res.body.expiresInSec
                        )
                    }
                    is LoopbackHttpOutcome.HttpError -> PairingResult.Error(res.message)
                    is LoopbackHttpOutcome.NetworkFailure -> PairingResult.Error(res.exception.message ?: "Ошибка сети")
                }
            } else {
                val api = apiServiceOverride ?: buildRetrofitApi(validation.normalizedBaseUrl)
                val response = api.initiatePhonePairing(req)
                val body = response.body()
                if (response.isSuccessful && body != null) {
                    credentialStore.savePendingPhonePairingSession(
                        sessionId = body.pairingSessionId,
                        verificationCode = body.userVerificationCode
                    )
                    logDiagnostic("INFO", "PAIRING", "PAIR_CODE_REQUESTED", "Запрошен проверочный код сопряжения", response.code())
                    PairingResult.CodeRequested(body.pairingSessionId, body.userVerificationCode, body.expiresInSec)
                } else {
                    PairingResult.Error("Не удалось запросить код на сервере (HTTP ${response.code()})")
                }
            }
        } catch (e: Exception) {
            val (_, msg) = classifyNetworkException(e)
            PairingResult.Error(msg)
        }
    }

    suspend fun pollPhonePairingConfirmation(): PairingResult {
        val config = credentialStore.state.value
        val sessionId = config.pendingPairingSessionId
            ?: return PairingResult.Error("Нет активной сессии сопряжения")
        val validation = TransportPolicyValidator.validate(config.serverUrl, config.allowCleartextLocalHttp)
        if (!validation.isValid) {
            return PairingResult.Error(validation.warningOrErrorRu ?: "Некорректный адрес сервера")
        }

        val req = PairPollRequestDto(
            pairingSessionId = sessionId,
            deviceId = config.deviceId
        )

        return try {
            if (validation.classification == RouteClassification.LOOPBACK_SANDBOX && apiServiceOverride == null) {
                when (val res = loopbackInspector.pollPhonePairing(req)) {
                    is LoopbackHttpOutcome.Success -> {
                        val token = res.body.deviceToken ?: return PairingResult.PendingApproval("Ожидание подтверждения в Web UI...")
                        val serverName = res.body.serverName ?: "Butler Home Server"
                        credentialStore.savePairingSuccess(
                            serverUrl = validation.normalizedBaseUrl,
                            serverName = serverName,
                            deviceToken = token
                        )
                        logDiagnostic("INFO", "PAIRING", "PAIR_POLL_APPROVED", "Сопряжение подтверждено владельцем в Web UI")
                        PairingResult.Paired(serverName, validation.normalizedBaseUrl)
                    }
                    is LoopbackHttpOutcome.HttpError -> PairingResult.Error(res.message)
                    is LoopbackHttpOutcome.NetworkFailure -> PairingResult.Error(res.exception.message ?: "Ошибка сети")
                }
            } else {
                val api = apiServiceOverride ?: buildRetrofitApi(validation.normalizedBaseUrl)
                val response = api.pollPhonePairingApproval(req)
                val body = response.body()
                if (response.code() == 202 || body?.status == "PENDING") {
                    PairingResult.PendingApproval("Ожидание подтверждения кода владельцем в Web UI Butler...")
                } else if (response.isSuccessful && body?.status == "APPROVED" && !body.deviceToken.isNullOrBlank()) {
                    val serverName = body.serverName ?: "Butler Server"
                    credentialStore.savePairingSuccess(
                        serverUrl = validation.normalizedBaseUrl,
                        serverName = serverName,
                        deviceToken = body.deviceToken
                    )
                    logDiagnostic("INFO", "PAIRING", "PAIR_POLL_APPROVED", "Сопряжение подтверждено владельцем", response.code())
                    PairingResult.Paired(serverName, validation.normalizedBaseUrl)
                } else {
                    PairingResult.Error("Сессия сопряжения отклонена или истекла (HTTP ${response.code()})")
                }
            }
        } catch (e: Exception) {
            val (_, msg) = classifyNetworkException(e)
            PairingResult.Error(msg)
        }
    }

    suspend fun pingServerStatus(): Pair<Boolean, String> {
        val config = credentialStore.state.value
        val validation = TransportPolicyValidator.validate(config.serverUrl, config.allowCleartextLocalHttp)
        if (!validation.isValid) {
            val msg = validation.warningOrErrorRu ?: "Некорректный адрес сервера"
            return false to msg
        }
        val token = credentialStore.getDecryptedDeviceToken()
        if (token.isNullOrBlank()) {
            return false to "Устройство не сопряжено (отсутствует токен устройства)"
        }

        val startMs = System.currentTimeMillis()
        return try {
            if (validation.classification == RouteClassification.LOOPBACK_SANDBOX && apiServiceOverride == null) {
                when (val res = loopbackInspector.checkStatus(config.deviceId, config.loopbackSimulationMode)) {
                    is LoopbackHttpOutcome.Success -> {
                        val latency = (System.currentTimeMillis() - startMs).coerceAtLeast(2L)
                        logDiagnostic("INFO", "NETWORK_SYNC", "PING_OK", "Связь с ${res.body.serverName} исправна (${latency} мс)", 200, null, latency)
                        true to "Связь исправна: ${res.body.serverName} (${latency} мс)"
                    }
                    is LoopbackHttpOutcome.HttpError -> {
                        if (res.httpCode == 401 || res.httpCode == 403) {
                            credentialStore.markTokenRevoked(res.message)
                        }
                        logDiagnostic("WARN", "NETWORK_SYNC", res.errorCode, res.message, res.httpCode)
                        false to res.message
                    }
                    is LoopbackHttpOutcome.NetworkFailure -> {
                        val (cat, msg) = classifyNetworkException(res.exception)
                        logDiagnostic("WARN", "NETWORK_SYNC", cat, msg)
                        false to msg
                    }
                }
            } else {
                val api = apiServiceOverride ?: buildRetrofitApi(validation.normalizedBaseUrl)
                val response = api.getDeviceStatus(
                    bearerToken = "Bearer $token",
                    deviceId = config.deviceId
                )
                val latency = (System.currentTimeMillis() - startMs).coerceAtLeast(1L)
                if (response.isSuccessful && response.body() != null) {
                    val serverName = response.body()!!.serverName
                    logDiagnostic("INFO", "NETWORK_SYNC", "PING_OK", "Связь с $serverName подтверждена (${latency} мс)", response.code(), null, latency)
                    true to "Связь с $serverName подтверждена (${latency} мс)"
                } else if (response.code() == 401 || response.code() == 403) {
                    val msg = "Токен устройства отозван на сервере (HTTP ${response.code()})"
                    credentialStore.markTokenRevoked(msg)
                    logDiagnostic("ERROR", "SECURITY", "PING_TOKEN_REVOKED", msg, response.code())
                    false to msg
                } else {
                    val msg = "Сервер ответил HTTP ${response.code()}"
                    logDiagnostic("WARN", "NETWORK_SYNC", "PING_HTTP_ERR", msg, response.code())
                    false to msg
                }
            }
        } catch (e: Exception) {
            val (cat, msg) = classifyNetworkException(e)
            logDiagnostic("WARN", "NETWORK_SYNC", cat, msg)
            false to msg
        }
    }

    suspend fun disconnectDeviceAndWipe() = syncMutex.withLock {
        val config = credentialStore.state.value
        val token = credentialStore.getDecryptedDeviceToken()
        val validation = TransportPolicyValidator.validate(config.serverUrl, config.allowCleartextLocalHttp)

        // Fence an in-flight send, then serialize key destruction with capture.
        // Wipe before awaiting remote I/O: new callbacks cannot enter a fresh
        // pairing's queue while the old server is unavailable.
        captureMutex.withLock {
            credentialStore.revokeAndClearCredentials()
            dao.clearEntireQueue()
        }

        if (!token.isNullOrBlank() && validation.isValid && validation.classification != RouteClassification.LOOPBACK_SANDBOX) {
            try {
                val api = apiServiceOverride ?: buildRetrofitApi(validation.normalizedBaseUrl)
                api.disconnectDevice(
                    bearerToken = "Bearer $token",
                    deviceId = config.deviceId,
                    request = DisconnectRequestDto(deviceId = config.deviceId)
                )
            } catch (_: Exception) {
                // Local revocation proceeds unconditionally even if server is unreachable
            }
        }

        logDiagnostic(
            level = "INFO",
            stage = "SECURITY",
            code = "DEVICE_DISCONNECTED_AND_KEY_ROTATED",
            summaryRu = "Устройство отключено владельцем; ключ Android Keystore уничтожен и перевыпущен"
        )
    }

    suspend fun clearDeliveredEvents(): Int {
        val deleted = dao.clearAllDeliveredEvents()
        logDiagnostic("INFO", "QUEUE", "DELIVERED_PURGED", "Очищено подтверждённых событий из локальной БД: $deleted")
        return deleted
    }

    suspend fun clearAllQueuedEvents() {
        dao.clearEntireQueue()
        credentialStore.resetDroppedOverflowCount()
        logDiagnostic("INFO", "QUEUE", "QUEUE_CLEARED_BY_OWNER", "Владелец полностью очистил локальную очередь событий")
    }

    suspend fun clearDiagnostics() {
        dao.clearDiagnosticLogs()
    }

    suspend fun logDiagnostic(
        level: String,
        stage: String,
        code: String,
        summaryRu: String,
        httpStatus: Int? = null,
        batchSize: Int? = null,
        latencyMs: Long? = null
    ) {
        dao.insertDiagnosticLog(
            DiagnosticLogEntity(
                timestampEpochMs = System.currentTimeMillis(),
                level = level,
                stage = stage,
                code = code,
                summaryRu = summaryRu,
                httpStatus = httpStatus,
                batchSize = batchSize,
                latencyMs = latencyMs
            )
        )
        dao.trimDiagnosticLogs()
    }

    private suspend fun updateAppRuleStats(packageName: String, observedAtMs: Long) {
        val existing = dao.getAppFilterRule(packageName) ?: return
        dao.upsertAppFilterRule(
            existing.copy(
                lastSeenNotificationEpochMs = observedAtMs,
                totalObservedCount = existing.totalObservedCount + 1
            )
        )
    }

    private data class ParsedPairingInput(val serverUrl: String, val code: String)

    private fun parsePairingInput(defaultServerUrl: String, rawInput: String): ParsedPairingInput {
        val trimmed = rawInput.trim()
        // Support QR URI format: butler://pair?url=https://...&code=849-204
        if (trimmed.startsWith("butler://pair", ignoreCase = true)) {
            val query = trimmed.substringAfter("?", "")
            val params = query.split("&").mapNotNull { part ->
                val idx = part.indexOf('=')
                if (idx > 0) part.substring(0, idx) to java.net.URLDecoder.decode(part.substring(idx + 1), "UTF-8")
                else null
            }.toMap()
            val url = params["url"]?.takeIf { it.isNotBlank() } ?: defaultServerUrl
            val code = params["code"] ?: ""
            return ParsedPairingInput(url, code)
        }
        // Support JSON QR string: {"url":"http://192.168.1.50:8080","code":"849-204"}
        if (trimmed.startsWith("{") && trimmed.endsWith("}")) {
            val urlMatch = Regex(""""url"\s*:\s*"([^"]+)"""").find(trimmed)?.groupValues?.get(1)
            val codeMatch = Regex(""""code"\s*:\s*"([^"]+)"""").find(trimmed)?.groupValues?.get(1)
            return ParsedPairingInput(
                serverUrl = urlMatch ?: defaultServerUrl,
                code = codeMatch ?: trimmed
            )
        }
        return ParsedPairingInput(defaultServerUrl, trimmed)
    }

    private fun classifyNetworkException(e: Exception): Pair<String, String> {
        return when (e) {
            is SSLException -> "TLS_ERROR" to "Ошибка проверки SSL/TLS сертификата"
            is UnknownHostException -> "DNS_UNREACHABLE" to "Не удалось разрешить имя хоста (DNS) или сервер недоступен"
            is SocketTimeoutException -> "NETWORK_TIMEOUT" to "Превышен тайм-аут ожидания ответа от сервера Butler"
            is IOException -> "NETWORK_IO_ERROR" to "Сетевая ошибка соединения с сервером"
            else -> "SYNC_UNEXPECTED" to "Ошибка при обмене с сервером: ${e.javaClass.simpleName}"
        }
    }

    private fun buildRetrofitApi(normalizedBaseUrl: String): ButlerMobileApiService = buildRetrofit(normalizedBaseUrl).create(ButlerMobileApiService::class.java)

    private fun buildRetrofit(normalizedBaseUrl: String): Retrofit {
        val okHttp = OkHttpClient.Builder()
            .connectTimeout(8, TimeUnit.SECONDS)
            .readTimeout(12, TimeUnit.SECONDS)
            .writeTimeout(12, TimeUnit.SECONDS)
            .followRedirects(false)
            .followSslRedirects(false)
            .build()

        val baseUrlWithSlash = if (normalizedBaseUrl.endsWith("/")) normalizedBaseUrl else "$normalizedBaseUrl/"
        return Retrofit.Builder()
            .baseUrl(baseUrlWithSlash)
            .client(okHttp)
            .addConverterFactory(MoshiConverterFactory.create(moshi))
            .build()
    }

    companion object {
        private val KNOWN_PACKAGE_LABELS = mapOf(
            "android" to "Система Android (OS)",
            "com.android.systemui" to "Интерфейс системы (System UI)",
            "com.android.settings" to "Настройки Android",
            "com.android.vending" to "Google Play Маркет",
            "com.google.android.gms" to "Сервисы Google Play",
            "com.google.android.gsf" to "Google Services Framework",
            "com.google.android.apps.messaging" to "Сообщения Google",
            "com.google.android.dialer" to "Телефон",
            "com.android.phone" to "Телефонный сервис",
            "com.android.providers.downloads" to "Менеджер загрузок",
            "com.android.chrome" to "Google Chrome",
            "com.google.android.gm" to "Gmail",
            "com.google.android.calendar" to "Google Календарь",
            "com.google.android.youtube" to "YouTube",
            "com.google.android.apps.maps" to "Google Карты",
            "com.google.android.apps.photos" to "Google Фото",
            "com.google.android.googlequicksearchbox" to "Приложение Google",
            "org.telegram.messenger" to "Telegram",
            "com.whatsapp" to "WhatsApp",
            "com.github.android" to "GitHub",
            "ru.sberbankmobile" to "СберБанк Онлайн",
            "com.idamob.tinkoff.android" to "Т-Банк (Тинькофф)",
            "ru.alfabank.mobile.android" to "Альфа-Банк",
            "ru.vtb24.mobilebanking.android" to "ВТБ Онлайн",
            "com.google.android.apps.authenticator2" to "Google Authenticator",
            "com.aistudio.butlercompanion.kxmzqp" to "Butler (Тест)",
            "com.example" to "Butler (Тест)"
        )

        private val GENERIC_SUFFIXES = setOf(
            "android", "mobile", "app", "apps", "client", "messenger", "release", "debug", "free", "pro"
        )

        fun inferAppLabel(packageName: String): String {
            val clean = packageName.trim()
            KNOWN_PACKAGE_LABELS[clean]?.let { return it }

            val segments = clean.split('.').filter { it.isNotBlank() }
            if (segments.isEmpty()) return clean

            // Walk backwards and pick the first non-generic segment (so com.github.android -> Github, not Android)
            val meaningful = segments.lastOrNull { it.lowercase() !in GENERIC_SUFFIXES }
                ?: segments.last()

            val formatted = meaningful.replace('_', ' ').replaceFirstChar { it.uppercase() }
            return if (clean.startsWith("com.android.") || clean.startsWith("com.google.android.")) {
                "$formatted (${segments.takeLast(2).joinToString(".")})"
            } else {
                formatted
            }
        }
    }
}
