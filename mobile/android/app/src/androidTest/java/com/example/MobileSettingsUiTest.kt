package com.example

import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.assertIsDisplayed
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
}
