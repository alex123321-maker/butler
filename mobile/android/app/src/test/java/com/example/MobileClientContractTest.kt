package com.example

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.example.data.local.ButlerDatabase
import com.example.data.remote.ButlerMobileApiService
import com.example.data.repository.ButlerSyncRepository
import com.example.domain.RawNotificationSnapshot
import com.example.model.*
import com.example.security.DeviceCredentialStore
import com.example.security.KeystoreCryptoManager
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.yield
import okhttp3.ResponseBody.Companion.toResponseBody
import org.junit.After
import org.junit.Assert.*
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import retrofit2.Response
import java.net.SocketTimeoutException

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [36])
class MobileClientContractTest {
    private lateinit var db: ButlerDatabase
    private lateinit var credentials: DeviceCredentialStore
    private lateinit var repo: ButlerSyncRepository
    private var permission = true
    private val api = FakeApi()

    @Before fun setup() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        val crypto = KeystoreCryptoManager("mobile_contract", allowInMemoryForTests = true)
        credentials = DeviceCredentialStore(context, crypto)
        credentials.revokeAndClearCredentials()
        credentials.setAppFilterMode(AppFilterMode.ALL_EXCEPT_EXCLUDED)
        credentials.setContentTransmissionEnabled(true)
        credentials.savePairingSuccess("https://butler.example", "fixture", "a".repeat(64))
        db = ButlerDatabase.createInMemory(context)
        repo = ButlerSyncRepository(db.butlerDao(), credentials, crypto,
            apiServiceOverride = api, notificationAccessGranted = { permission })
    }
    @After fun close() = db.close()

    private suspend fun insert(id: String = "event_contract_000001") {
        repo.ingestRawNotification(RawNotificationSnapshot(id, "dev.butler.fixture",
            postedAtEpochMs = System.currentTimeMillis(), text = id), deterministicEventId = id)
    }

    @Test fun partialAndInvalidAcksNeverConfirmUnacknowledgedIds() = runBlocking {
        insert("event_contract_000001"); insert("event_contract_000002")
        api.handler = { Response.success(202, NotificationBatchResponseDto(acceptedIds = listOf(it.events.first().deviceEventId))) }
        repo.syncPendingQueueBatch(true)
        assertEquals("DELIVERED", db.butlerDao().getEventById("event_contract_000001")!!.deliveryState)
        assertEquals("PENDING", db.butlerDao().getEventById("event_contract_000002")!!.deliveryState)
        api.handler = { Response.success(NotificationBatchResponseDto(acceptedIds = listOf("foreign_event_000001"))) }
        repo.syncPendingQueueBatch(true)
        assertEquals("PENDING", db.butlerDao().getEventById("event_contract_000002")!!.deliveryState)
        api.handler = { Response.success(NotificationBatchResponseDto(acceptedIds = listOf(it.events.single().deviceEventId), duplicateIds = listOf(it.events.single().deviceEventId))) }
        repo.syncPendingQueueBatch(true)
        assertEquals("PENDING", db.butlerDao().getEventById("event_contract_000002")!!.deliveryState)
    }

    @Test fun lostResponseRetriesIdenticalEventAndAcceptsDuplicateAck() = runBlocking {
        insert()
        var committed: MobileNotificationEventDto? = null
        api.handler = { committed = it.events.single(); throw SocketTimeoutException("synthetic private error") }
        repo.syncPendingQueueBatch(true)
        assertEquals("PENDING", db.butlerDao().getEventById("event_contract_000001")!!.deliveryState)
        assertFalse(db.butlerDao().observeDiagnosticLogs().first().any { it.summaryRu.contains("synthetic private error") })
        api.handler = { assertEquals(committed, it.events.single()); Response.success(NotificationBatchResponseDto(duplicateIds = listOf(it.events.single().deviceEventId))) }
        repo.syncPendingQueueBatch(true)
        assertEquals("DELIVERED", db.butlerDao().getEventById("event_contract_000001")!!.deliveryState)
    }

    @Test fun retryableHttpFailuresPreserveEncryptedQueueAndBackoff() = runBlocking {
        for (code in listOf(400, 423, 429, 500, 503)) {
            val id = "http_event_${code}_000001"
            insert(id)
            api.handler = { Response.error(code, "arbitrary sensitive server error".toResponseBody()) }
            repo.syncPendingQueueBatch(true)
            val row = db.butlerDao().getEventById(id)!!
            assertEquals("PENDING", row.deliveryState)
            assertNotNull(row.encryptedPayloadBlob)
            assertTrue(row.nextAttemptAtEpochMs > System.currentTimeMillis())
            assertFalse(row.lastErrorReason.orEmpty().contains("arbitrary sensitive"))
        }
    }

    @Test fun authenticationRevocationStopsFurtherRequestsAndConflictStopsRetry() = runBlocking {
        for (code in listOf(401, 403)) {
            credentials.savePairingSuccess("https://butler.example", "fixture", "a".repeat(64))
            credentials.setSyncPaused(false)
            insert("auth_event_${code}_000001")
            api.handler = { Response.error(code, "secret".toResponseBody()) }
            repo.syncPendingQueueBatch(true)
            assertTrue(credentials.state.value.isTokenRevoked)
            val calls = api.calls
            repo.syncPendingQueueBatch(true)
            assertEquals(calls, api.calls)
        }
        credentials.savePairingSuccess("https://butler.example", "fixture", "a".repeat(64))
        credentials.setSyncPaused(false)
        api.handler = { Response.error(409, "secret".toResponseBody()) }
        repo.syncPendingQueueBatch(true)
        assertEquals("CONFLICT_ERROR", db.butlerDao().getEventById("auth_event_401_000001")!!.deliveryState)
    }

    @Test fun permissionPauseAndAllowlistGateCollectionAndTransmission() = runBlocking {
        insert()
        permission = false
        insert("permission_event_000001")
        repo.syncPendingQueueBatch(true)
        assertNull(db.butlerDao().getEventById("permission_event_000001"))
        assertEquals(0, api.calls)
        permission = true
        credentials.setSyncPaused(true)
        insert("paused_event_000001")
        repo.syncPendingQueueBatch(true)
        assertNull(db.butlerDao().getEventById("paused_event_000001"))
        assertEquals(0, api.calls)
        credentials.setSyncPaused(false)
        credentials.setAppFilterMode(AppFilterMode.ONLY_INCLUDED)
        insert("unknown_app_event_000001")
        assertNull(db.butlerDao().getEventById("unknown_app_event_000001"))
        credentials.updateServerUrl("https://untrusted.example", false)
        assertEquals("https://butler.example", credentials.state.value.serverUrl)
    }

    @Test fun concurrentCallbacksDeduplicateAndRemovalAllowsFreshPost() = runBlocking {
        val snapshot = RawNotificationSnapshot("same-card", "dev.butler.fixture", postedAtEpochMs = System.currentTimeMillis(), text = "synthetic")
        (1..12).map { async { repo.ingestRawNotification(snapshot) } }.awaitAll()
        assertEquals(1, db.butlerDao().getTotalEventCount())
        repo.ingestNotificationRemoved("same-card", "dev.butler.fixture", null, snapshot.postedAtEpochMs)
        repo.ingestRawNotification(snapshot.copy(observedAtEpochMs = System.currentTimeMillis() + 1))
        assertEquals(3, db.butlerDao().getTotalEventCount())
    }

    @Test fun disconnectSerializesWithInFlightSendAndClearsBeforeRePairing() = runBlocking {
        insert()
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        api.handler = {
            entered.complete(Unit)
            release.await()
            Response.success(NotificationBatchResponseDto(acceptedIds = it.events.map { e -> e.deviceEventId }))
        }
        val send = async { repo.syncPendingQueueBatch(true) }
        entered.await()
        val disconnect = async { repo.disconnectDeviceAndWipe() }
        yield()
        assertFalse("Disconnect must wait for in-flight send", disconnect.isCompleted)
        release.complete(Unit)
        send.await(); disconnect.await()
        assertFalse(credentials.state.value.isPaired)
        assertEquals(0, db.butlerDao().getTotalEventCount())
        credentials.savePairingSuccess("https://butler.example", "new fixture", "b".repeat(64))
        repo.syncPendingQueueBatch(true)
        assertEquals(1, api.calls)
    }

    private class FakeApi : ButlerMobileApiService {
        var calls = 0
        var handler: suspend (NotificationBatchRequestDto) -> Response<NotificationBatchResponseDto> = {
            Response.success(NotificationBatchResponseDto(acceptedIds = it.events.map { e -> e.deviceEventId }))
        }
        override suspend fun postNotificationBatch(bearerToken: String, deviceId: String, schemaVersion: Int, batch: NotificationBatchRequestDto): Response<NotificationBatchResponseDto> { calls++; return handler(batch) }
        override suspend fun exchangePairingCode(request: PairExchangeRequestDto): Response<PairExchangeResponseDto> = error("unused")
        override suspend fun initiatePhonePairing(request: PairRequestInitDto): Response<PairRequestInitResponseDto> = error("unused")
        override suspend fun pollPhonePairingApproval(request: PairPollRequestDto): Response<PairPollResponseDto> = error("unused")
        override suspend fun getDeviceStatus(bearerToken: String, deviceId: String, schemaVersion: Int): Response<DeviceStatusResponseDto> = error("unused")
        override suspend fun disconnectDevice(bearerToken: String, deviceId: String, request: DisconnectRequestDto): Response<Unit> = Response.success(Unit)
    }
}
