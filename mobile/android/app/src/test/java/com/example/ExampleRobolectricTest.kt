package com.example

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.example.data.local.ButlerDatabase
import com.example.data.repository.ButlerSyncRepository
import com.example.data.repository.SyncBatchResult
import com.example.domain.RawNotificationSnapshot
import com.example.model.LoopbackSimulationMode
import com.example.security.DeviceCredentialStore
import com.example.security.KeystoreCryptoManager
import com.example.security.TransportPolicyValidator
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [36])
class ExampleRobolectricTest {

    private lateinit var context: Context
    private lateinit var cryptoManager: KeystoreCryptoManager
    private lateinit var credentialStore: DeviceCredentialStore
    private lateinit var database: ButlerDatabase
    private lateinit var repository: ButlerSyncRepository

    @Before
    fun setUp() {
        context = ApplicationProvider.getApplicationContext()
        cryptoManager = KeystoreCryptoManager("test_butler_key", allowInMemoryForTests = true)
        credentialStore = DeviceCredentialStore(context, cryptoManager)
        credentialStore.revokeAndClearCredentials()
        credentialStore.resetDroppedOverflowCount()
        database = ButlerDatabase.createInMemory(context)
        repository = ButlerSyncRepository(
            dao = database.butlerDao(),
            credentialStore = credentialStore,
            cryptoManager = cryptoManager
        )
    }

    @After
    fun tearDown() {
        database.close()
    }

    @Test
    fun `encrypts payload in Room SQLite and delivers batch with per-ID ACK`() = runBlocking {
        repository.ensureDefaultAppRulesSeeded()
        repository.pairWithServerCodeOrQr(
            rawServerUrl = TransportPolicyValidator.LOOPBACK_BASE_URL,
            allowCleartextHttp = true,
            codeOrQrPayload = "849-204"
        )
        assertTrue(credentialStore.state.value.isPaired)

        val secretBody = "Секретное сообщение в чате, которое должно быть зашифровано в SQLite"
        val inserted = repository.ingestRawNotification(
            snapshot = RawNotificationSnapshot(
                notificationKey = "0|org.telegram.messenger|10|null|100",
                packageName = "org.telegram.messenger",
                postedAtEpochMs = 1790000100000L,
                title = "Приватный чат",
                text = secretBody
            ),
            deterministicEventId = "evt-encrypted-1"
        )
        assertNotNull(inserted)

        // Verify raw SQLite column does NOT contain plaintext
        val rawEntity = database.butlerDao().getEventById("evt-encrypted-1")!!
        assertNotNull(rawEntity.encryptedPayloadBlob)
        assertFalse(
            "SQLite blob must never contain cleartext notification body",
            rawEntity.encryptedPayloadBlob!!.contains(secretBody)
        )

        // Verify decryption roundtrip works
        val decrypted = repository.decryptPayloadForUi(rawEntity)
        assertEquals(secretBody, decrypted?.text)

        // Deliver batch and verify per-ID ACK transitions state to DELIVERED
        credentialStore.setLoopbackSimulationMode(LoopbackSimulationMode.NORMAL_200_ACK)
        val syncResult = repository.syncPendingQueueBatch(ignoreBackoffTimers = true)
        assertTrue(syncResult is SyncBatchResult.Success)
        assertEquals(1, (syncResult as SyncBatchResult.Success).acceptedCount)

        val updatedEntity = database.butlerDao().getEventById("evt-encrypted-1")!!
        assertEquals("DELIVERED", updatedEntity.deliveryState)
    }

    @Test
    fun `detects 409 ID content hash conflict without overwriting and halts on 401 token revocation`() = runBlocking {
        repository.ensureDefaultAppRulesSeeded()
        repository.pairWithServerCodeOrQr(
            rawServerUrl = TransportPolicyValidator.LOOPBACK_BASE_URL,
            allowCleartextHttp = true,
            codeOrQrPayload = "849-204"
        )

        // Pre-seed server with same device_event_id but different content_hash
        repository.loopbackInspector.recordPreExistingServerHash(
            deviceEventId = "evt-conflict-99",
            contentHash = "0000000000000000000000000000000000000000000000000000000000000000"
        )

        repository.ingestRawNotification(
            snapshot = RawNotificationSnapshot(
                notificationKey = "0|org.telegram.messenger|99|null|100",
                packageName = "org.telegram.messenger",
                postedAtEpochMs = 1790000200000L,
                title = "Конфликтная карточка",
                text = "Новое содержимое под старым ID"
            ),
            deterministicEventId = "evt-conflict-99"
        )

        val syncRes = repository.syncPendingQueueBatch(ignoreBackoffTimers = true)
        assertTrue(syncRes is SyncBatchResult.Success)
        assertEquals(1, (syncRes as SyncBatchResult.Success).conflictCount)

        val conflictedEntity = database.butlerDao().getEventById("evt-conflict-99")!!
        assertEquals("CONFLICT_ERROR", conflictedEntity.deliveryState)

        // Now test 401 Token Revocation: must preserve pending event, mark token revoked, and clear decrypted token
        repository.ingestRawNotification(
            snapshot = RawNotificationSnapshot(
                notificationKey = "0|org.telegram.messenger|100|null|100",
                packageName = "org.telegram.messenger",
                postedAtEpochMs = 1790000300000L,
                title = "Событие до отзыва токена",
                text = "Останется в очереди"
            ),
            deterministicEventId = "evt-before-revoke"
        )

        credentialStore.setLoopbackSimulationMode(LoopbackSimulationMode.UNAUTHORIZED_401)
        val revokeRes = repository.syncPendingQueueBatch(ignoreBackoffTimers = true)
        assertTrue(revokeRes is SyncBatchResult.Failure)
        assertTrue(credentialStore.state.value.isTokenRevoked)
        assertNull(credentialStore.getDecryptedDeviceToken())

        val preservedEntity = database.butlerDao().getEventById("evt-before-revoke")!!
        assertEquals("PENDING", preservedEntity.deliveryState)
    }

    @Test
    fun `enforces queue capacity limit and increments droppedOverflowCount instead of silent drop`() = runBlocking {
        repository.ensureDefaultAppRulesSeeded()
        repository.pairWithServerCodeOrQr(
            rawServerUrl = TransportPolicyValidator.LOOPBACK_BASE_URL,
            allowCleartextHttp = true,
            codeOrQrPayload = "849-204"
        )
        // Set minimum queue count limit (50) and insert 52 pending events while offline
        credentialStore.updateQueueLimits(maxCount = 50, maxBytes = 10 * 1024 * 1024L, maxAgeDays = 14)

        val baseTimeMs = System.currentTimeMillis()
        for (i in 1..52) {
            repository.ingestRawNotification(
                snapshot = RawNotificationSnapshot(
                    notificationKey = "0|org.telegram.messenger|$i|null|100",
                    packageName = "org.telegram.messenger",
                    postedAtEpochMs = baseTimeMs + i,
                    observedAtEpochMs = baseTimeMs + i,
                    title = "Уведомление #$i",
                    text = "Текст события #$i"
                ),
                deterministicEventId = "evt-cap-$i"
            )
        }

        val pendingCount = repository.pendingCount.first()
        assertEquals(50, pendingCount)
        assertEquals(2L, credentialStore.state.value.droppedOverflowCount)
    }
}
