package com.example.data.remote

import com.example.model.DeviceStatusResponseDto
import com.example.model.LoopbackSimulationMode
import com.example.model.NotificationBatchRequestDto
import com.example.model.NotificationBatchResponseDto
import com.example.model.PairExchangeRequestDto
import com.example.model.PairExchangeResponseDto
import com.example.model.PairPollRequestDto
import com.example.model.PairPollResponseDto
import com.example.model.PairRequestInitDto
import com.example.model.PairRequestInitResponseDto
import com.example.model.RejectedEventDto
import java.io.IOException
import java.net.SocketTimeoutException
import java.time.Instant
import java.util.UUID
import java.util.concurrent.ConcurrentHashMap
import javax.net.ssl.SSLHandshakeException

sealed class LoopbackHttpOutcome<out T> {
    data class Success<T>(val httpCode: Int, val body: T) : LoopbackHttpOutcome<T>()
    data class HttpError(val httpCode: Int, val errorCode: String, val message: String, val retryAfterSec: Int? = null) :
        LoopbackHttpOutcome<Nothing>()
    data class NetworkFailure(val exception: IOException) : LoopbackHttpOutcome<Nothing>()
}

/**
 * Built-in Contract v1 Loopback Inspector that faithfully implements the Butler Go server's
 * idempotency, per-ID ACK, duplicate detection, and hash conflict (`409` / `conflict_ids`) rules.
 *
 * Allows verifying all Issue #57 edge cases on-device or in tests even before the Go backend
 * is deployed, while real HTTP urls (`https://...`, `http://192.168.x.x`, `http://100.x.y.z`)
 * use the live Retrofit OkHttp stack.
 */
class LoopbackContractInspector {

    // Maps device_event_id -> content_hash already committed by the server
    private val committedEventHashes = ConcurrentHashMap<String, String>()

    fun exchangeCode(request: PairExchangeRequestDto): LoopbackHttpOutcome<PairExchangeResponseDto> {
        val cleanCode = request.pairingCode.trim()
        if (cleanCode.length < 4 || cleanCode.equals("INVALID", ignoreCase = true)) {
            return LoopbackHttpOutcome.HttpError(
                httpCode = 401,
                errorCode = "INVALID_PAIRING_CODE",
                message = "Код сопряжения недействителен или истёк"
            )
        }
        val token = "btl_dev_${UUID.randomUUID().toString().replace("-", "").take(24)}"
        return LoopbackHttpOutcome.Success(
            httpCode = 200,
            body = PairExchangeResponseDto(
                deviceId = request.deviceId,
                deviceToken = token,
                serverName = "Butler Core v1 (Loopback Inspector)",
                pairedAt = Instant.now().toString()
            )
        )
    }

    fun requestPhonePairing(request: PairRequestInitDto): LoopbackHttpOutcome<PairRequestInitResponseDto> {
        val codePart1 = (100..999).random()
        val codePart2 = (100..999).random()
        return LoopbackHttpOutcome.Success(
            httpCode = 200,
            body = PairRequestInitResponseDto(
                pairingSessionId = "ps_${UUID.randomUUID().toString().take(12)}",
                userVerificationCode = "$codePart1-$codePart2",
                expiresInSec = 300
            )
        )
    }

    fun pollPhonePairing(request: PairPollRequestDto): LoopbackHttpOutcome<PairPollResponseDto> {
        val token = "btl_dev_${UUID.randomUUID().toString().replace("-", "").take(24)}"
        return LoopbackHttpOutcome.Success(
            httpCode = 200,
            body = PairPollResponseDto(
                status = "APPROVED",
                deviceToken = token,
                serverName = "Butler Core v1 (Web UI Confirmed)"
            )
        )
    }

    fun checkStatus(
        deviceId: String,
        mode: LoopbackSimulationMode
    ): LoopbackHttpOutcome<DeviceStatusResponseDto> {
        return when (mode) {
            LoopbackSimulationMode.UNAUTHORIZED_401 -> LoopbackHttpOutcome.HttpError(
                httpCode = 401,
                errorCode = "DEVICE_TOKEN_REVOKED",
                message = "Удостоверение устройства отозвано владельцем на сервере"
            )
            LoopbackSimulationMode.TLS_ERROR -> LoopbackHttpOutcome.NetworkFailure(
                SSLHandshakeException("Не удалось проверить цепочку сертификатов TLS сервера Butler")
            )
            LoopbackSimulationMode.NETWORK_TIMEOUT -> LoopbackHttpOutcome.NetworkFailure(
                SocketTimeoutException("Истёк тайм-аут ожидания ответа от сервера Butler (10 000 мс)")
            )
            LoopbackSimulationMode.SERVER_ERROR_503 -> LoopbackHttpOutcome.HttpError(
                httpCode = 503,
                errorCode = "STORAGE_UNAVAILABLE",
                message = "PostgreSQL временно недоступен"
            )
            else -> LoopbackHttpOutcome.Success(
                httpCode = 200,
                body = DeviceStatusResponseDto(
                    deviceId = deviceId,
                    status = "ACTIVE",
                    serverName = "Butler Core v1 (Loopback Inspector)",
                    serverTimeEpochMs = System.currentTimeMillis(),
                    maxBatchSize = 50
                )
            )
        }
    }

    fun processBatch(
        batch: NotificationBatchRequestDto,
        mode: LoopbackSimulationMode
    ): LoopbackHttpOutcome<NotificationBatchResponseDto> {
        when (mode) {
            LoopbackSimulationMode.UNAUTHORIZED_401 -> {
                return LoopbackHttpOutcome.HttpError(
                    httpCode = 401,
                    errorCode = "DEVICE_TOKEN_REVOKED",
                    message = "Токен устройства отозван или недействителен"
                )
            }
            LoopbackSimulationMode.RATE_LIMIT_429 -> {
                return LoopbackHttpOutcome.HttpError(
                    httpCode = 429,
                    errorCode = "RATE_LIMITED",
                    message = "Превышен лимит частоты пакетов (429 Too Many Requests)",
                    retryAfterSec = 15
                )
            }
            LoopbackSimulationMode.SERVER_ERROR_503 -> {
                return LoopbackHttpOutcome.HttpError(
                    httpCode = 503,
                    errorCode = "POSTGRES_COMMIT_FAILED",
                    message = "Сервер не смог зафиксировать транзакцию (503)"
                )
            }
            LoopbackSimulationMode.TLS_ERROR -> {
                return LoopbackHttpOutcome.NetworkFailure(
                    SSLHandshakeException("Ошибка проверки TLS сертификата при отправке пакета")
                )
            }
            LoopbackSimulationMode.NETWORK_TIMEOUT -> {
                return LoopbackHttpOutcome.NetworkFailure(
                    SocketTimeoutException("Тайм-аут чтения HTTP-ответа при отправке пакета")
                )
            }
            LoopbackSimulationMode.CONFLICT_409 -> {
                val conflictIds = batch.events.map { it.deviceEventId }
                val rejected = batch.events.map {
                    RejectedEventDto(
                        deviceEventId = it.deviceEventId,
                        errorCode = "ID_CONTENT_HASH_CONFLICT",
                        reason = "Конфликт: device_event_id уже зафиксирован на сервере с другим content_hash"
                    )
                }
                return LoopbackHttpOutcome.Success(
                    httpCode = 207,
                    body = NotificationBatchResponseDto(
                        acceptedIds = emptyList(),
                        duplicateIds = emptyList(),
                        conflictIds = conflictIds,
                        rejected = rejected,
                        serverTimeEpochMs = System.currentTimeMillis()
                    )
                )
            }
            LoopbackSimulationMode.PARTIAL_207_ACK -> {
                val accepted = mutableListOf<String>()
                val duplicates = mutableListOf<String>()
                batch.events.forEachIndexed { index, event ->
                    val existingHash = committedEventHashes[event.deviceEventId]
                    if (existingHash != null || index % 2 == 1) {
                        duplicates.add(event.deviceEventId)
                    } else {
                        committedEventHashes[event.deviceEventId] = event.contentHash
                        accepted.add(event.deviceEventId)
                    }
                }
                return LoopbackHttpOutcome.Success(
                    httpCode = 207,
                    body = NotificationBatchResponseDto(
                        acceptedIds = accepted,
                        duplicateIds = duplicates,
                        conflictIds = emptyList(),
                        rejected = emptyList(),
                        serverTimeEpochMs = System.currentTimeMillis()
                    )
                )
            }
            LoopbackSimulationMode.NORMAL_200_ACK -> {
                val accepted = mutableListOf<String>()
                val duplicates = mutableListOf<String>()
                val conflicts = mutableListOf<String>()
                val rejected = mutableListOf<RejectedEventDto>()

                for (event in batch.events) {
                    val previousHash = committedEventHashes.putIfAbsent(event.deviceEventId, event.contentHash)
                    when {
                        previousHash == null -> accepted.add(event.deviceEventId)
                        previousHash == event.contentHash -> duplicates.add(event.deviceEventId)
                        else -> {
                            conflicts.add(event.deviceEventId)
                            rejected.add(
                                RejectedEventDto(
                                    deviceEventId = event.deviceEventId,
                                    errorCode = "ID_CONTENT_HASH_CONFLICT",
                                    reason = "Тот же device_event_id отправлен с изменённым content_hash"
                                )
                            )
                        }
                    }
                }

                return LoopbackHttpOutcome.Success(
                    httpCode = if (conflicts.isEmpty()) 200 else 207,
                    body = NotificationBatchResponseDto(
                        acceptedIds = accepted,
                        duplicateIds = duplicates,
                        conflictIds = conflicts,
                        rejected = rejected,
                        serverTimeEpochMs = System.currentTimeMillis()
                    )
                )
            }
        }
    }

    fun recordPreExistingServerHash(deviceEventId: String, contentHash: String) {
        committedEventHashes[deviceEventId] = contentHash
    }
}
