package com.example

import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.Context
import androidx.core.app.NotificationCompat
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.example.service.ButlerNotificationListenerService
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import org.junit.Assert.*
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith

/** Run seed with receiver stopped; kill app, restart receiver, then run drain. */
@RunWith(AndroidJUnit4::class)
class MobileOfflineAcceptanceTest {
    @Test fun durableOfflineQueueAndBackgroundRecovery() = runBlocking {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val phase = InstrumentationRegistry.getArguments().getString("mobile_phase")
        assumeTrue("External failure procedure required", phase == "seed" || phase == "drain")
        val app = instrumentation.targetContext.applicationContext as ButlerApplication
        assertTrue("Real pairing from live acceptance required", app.credentialStore.state.value.isPaired)
        val repo = app.syncRepository
        val manager = app.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        fun shell(command: String) { instrumentation.uiAutomation.executeShellCommand(command).use { fd -> java.io.FileInputStream(fd.fileDescriptor).use { it.readBytes() } } }
        shell("cmd notification allow_listener ${app.packageName}/com.example.service.ButlerNotificationListenerService")
        suspend fun await(label: String, check: suspend () -> Boolean) {
            repeat(160) { if (check()) return; delay(250) }
            val events = app.database.butlerDao().observeRecentQueueEvents().first()
            fail("$label timed out; paired=${app.credentialStore.state.value.isPaired}; listener=${ButlerNotificationListenerService.isListenerConnected.value}; pending=${repo.pendingCount.first()}; events=${events.map { "${it.changeType}:${it.deliveryState}:${it.notificationKey.contains("|5857|")}" }}")
        }
        await("listener connected") { ButlerNotificationListenerService.isListenerConnected.value }
        if (phase == "seed") {
            val oldIds = app.database.butlerDao().observeRecentQueueEvents().first().map { it.deviceEventId }.toSet()
            manager.createNotificationChannel(NotificationChannel("offline-acceptance", "Synthetic offline test", NotificationManager.IMPORTANCE_LOW))
            manager.notify(5857, NotificationCompat.Builder(app, "offline-acceptance").setSmallIcon(R.drawable.ic_launcher_foreground).setContentTitle("Synthetic offline #57").setContentText("Synthetic offline attempt ${System.currentTimeMillis()}").build())
            await("offline event persisted") { app.database.butlerDao().observeRecentQueueEvents().first().any { it.notificationKey.contains("|5857|") && it.deviceEventId !in oldIds } }
            repo.syncPendingQueueBatch(ignoreBackoffTimers = true)
            val events = app.database.butlerDao().observeRecentQueueEvents().first().filter { it.notificationKey.contains("|5857|") && it.deviceEventId !in oldIds }
            assertTrue(events.isNotEmpty())
            assertTrue(events.all { it.deliveryState == "PENDING" && it.encryptedPayloadBlob != null })
        } else {
            // No direct sync call: actual persisted WorkManager must drain it.
            await("background delivery") { val events = app.database.butlerDao().observeRecentQueueEvents().first().filter { it.notificationKey.contains("|5857|") }; events.isNotEmpty() && events.all { it.deliveryState == "DELIVERED" } }
            manager.cancel(5857)
        }
    }
}
