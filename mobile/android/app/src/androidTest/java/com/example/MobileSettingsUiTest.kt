package com.example

import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.assertIsDisplayed
import androidx.test.platform.app.InstrumentationRegistry
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import org.junit.Assert.*
import org.junit.Assume.assumeTrue
import org.junit.Rule
import org.junit.Test

class MobileSettingsUiTest {
    @get:Rule val compose = createAndroidComposeRule<MainActivity>()
    @Test fun settingsFiltersAndQueueAreReachable() {
        compose.onNodeWithTag("bottom_navigation_bar").assertIsDisplayed()
        compose.onNodeWithTag("nav_tab_pairing").performClick()
        compose.onNodeWithTag("server_url_input").assertIsDisplayed()
        compose.onNodeWithTag("nav_tab_filters").performClick()
        compose.onNodeWithTag("nav_tab_queue").performClick()
        compose.onNodeWithTag("nav_tab_dashboard").performClick()
        compose.onNodeWithTag("hero_status_card").assertIsDisplayed()
    }

    @Test fun explicitDisconnectWipesKeyQueueAndRevokesServerDevice() {
        assumeTrue("Only the disposable paired phone fixture", InstrumentationRegistry.getArguments().getString("mobile_disconnect") == "true")
        val app = InstrumentationRegistry.getInstrumentation().targetContext.applicationContext as ButlerApplication
        assertTrue(app.credentialStore.state.value.isPaired)
        val started = System.currentTimeMillis()
        compose.onNodeWithTag("nav_tab_pairing").performClick()
        compose.onNodeWithTag("disconnect_device_button").performScrollTo().performClick()
        compose.waitUntil(20_000) {
            runBlocking { app.database.butlerDao().observeDiagnosticLogs().first().any { it.code == "DEVICE_DISCONNECTED_AND_KEY_ROTATED" && it.timestampEpochMs >= started } }
        }
        assertFalse(app.credentialStore.state.value.isPaired)
        assertNull(app.credentialStore.getDecryptedDeviceToken())
        assertEquals(0, runBlocking { app.database.butlerDao().getTotalEventCount() })
        // The host separately checks owner GET devices for REVOKED; no owner
        // proof is ever passed to this test/application.
    }
}
