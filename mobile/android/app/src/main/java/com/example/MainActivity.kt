package com.example

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.provider.Settings
import androidx.activity.ComponentActivity
import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Apps
import androidx.compose.material.icons.filled.CloudQueue
import androidx.compose.material.icons.filled.Inbox
import androidx.compose.material.icons.filled.Tune
import androidx.compose.material3.Badge
import androidx.compose.material3.BadgedBox
import androidx.compose.material3.CenterAlignedTopAppBar
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.NavigationBarItemDefaults
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.content.ContextCompat
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.example.ui.AppFiltersScreen
import com.example.ui.ButlerTab
import com.example.ui.ButlerViewModel
import com.example.ui.DashboardScreen
import com.example.ui.PairingScreen
import com.example.ui.QueueAndDiagnosticsScreen
import com.example.ui.theme.MyApplicationTheme
import com.example.ui.theme.TelegramBlue

class MainActivity : ComponentActivity() {

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()

        val butlerApp = application as ButlerApplication

        setContent {
            MyApplicationTheme {
                val vm: ButlerViewModel = viewModel(
                    factory = ButlerViewModel.provideFactory(butlerApp)
                )
                ButlerCompanionApp(
                    viewModel = vm,
                    onOpenNotificationListenerSettings = {
                        openSystemSettings(Settings.ACTION_NOTIFICATION_LISTENER_SETTINGS)
                    },
                    onOpenBatteryOptimizationSettings = {
                        openSystemSettings(Settings.ACTION_IGNORE_BATTERY_OPTIMIZATION_SETTINGS)
                    }
                )
            }
        }
    }

    override fun onResume() {
        super.onResume()
        val butlerApp = application as? ButlerApplication ?: return
        butlerApp.networkMonitor.refreshSnapshot()
    }

    private fun openSystemSettings(action: String) {
        try {
            val intent = Intent(action).apply {
                addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            }
            startActivity(intent)
        } catch (_: Exception) {
            try {
                startActivity(Intent(Settings.ACTION_SETTINGS).apply {
                    addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
                })
            } catch (_: Exception) {
                // Ignore in headless test environment
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ButlerCompanionApp(
    viewModel: ButlerViewModel,
    onOpenNotificationListenerSettings: () -> Unit,
    onOpenBatteryOptimizationSettings: () -> Unit
) {
    val context = LocalContext.current
    val selectedTab by viewModel.selectedTab.collectAsStateWithLifecycle()
    val bannerMessage by viewModel.statusBannerMessage.collectAsStateWithLifecycle()
    val isSyncingNow by viewModel.isSyncingNow.collectAsStateWithLifecycle()
    val hasNotificationAccess by viewModel.hasNotificationAccess.collectAsStateWithLifecycle()
    val isListenerConnected by viewModel.isListenerServiceConnected.collectAsStateWithLifecycle()
    val deviceConfig by viewModel.deviceConfig.collectAsStateWithLifecycle()
    val environmentState by viewModel.environmentState.collectAsStateWithLifecycle()
    val queueMetrics by viewModel.queueMetrics.collectAsStateWithLifecycle()
    val recentQueueEvents by viewModel.recentQueueEvents.collectAsStateWithLifecycle()
    val appFilterRules by viewModel.appFilterRules.collectAsStateWithLifecycle()
    val diagnosticLogs by viewModel.diagnosticLogs.collectAsStateWithLifecycle()

    val postNotificationPermissionLauncher = rememberLauncherForActivityResult(
        contract = ActivityResultContracts.RequestPermission()
    ) { _ ->
        viewModel.postRealSystemTestNotification()
    }

    BackHandler(enabled = selectedTab != ButlerTab.DASHBOARD) {
        viewModel.selectTab(ButlerTab.DASHBOARD)
    }

    val topBarTitle = when (selectedTab) {
        ButlerTab.DASHBOARD -> "Butler"
        ButlerTab.PAIRING -> "Сервер"
        ButlerTab.FILTERS -> "Приложения"
        ButlerTab.QUEUE -> "Очередь"
    }

    val navItemColors = NavigationBarItemDefaults.colors(
        selectedIconColor = TelegramBlue,
        selectedTextColor = TelegramBlue,
        indicatorColor = Color.Transparent,
        unselectedIconColor = MaterialTheme.colorScheme.onSurfaceVariant,
        unselectedTextColor = MaterialTheme.colorScheme.onSurfaceVariant
    )

    Scaffold(
        modifier = Modifier.fillMaxSize(),
        contentWindowInsets = WindowInsets.safeDrawing,
        containerColor = MaterialTheme.colorScheme.background,
        topBar = {
            CenterAlignedTopAppBar(
                title = {
                    Text(
                        text = topBarTitle,
                        style = MaterialTheme.typography.titleMedium.copy(
                            fontSize = 17.sp,
                            fontWeight = FontWeight.SemiBold
                        )
                    )
                },
                colors = TopAppBarDefaults.centerAlignedTopAppBarColors(
                    containerColor = MaterialTheme.colorScheme.background
                )
            )
        },
        bottomBar = {
            NavigationBar(
                containerColor = MaterialTheme.colorScheme.surface,
                tonalElevation = 0.dp,
                modifier = Modifier.testTag("bottom_navigation_bar")
            ) {
                NavigationBarItem(
                    selected = selectedTab == ButlerTab.DASHBOARD,
                    onClick = { viewModel.selectTab(ButlerTab.DASHBOARD) },
                    icon = { Icon(Icons.Default.Tune, contentDescription = "Обзор") },
                    label = { Text("Обзор") },
                    colors = navItemColors,
                    modifier = Modifier.testTag("nav_tab_dashboard")
                )
                NavigationBarItem(
                    selected = selectedTab == ButlerTab.PAIRING,
                    onClick = { viewModel.selectTab(ButlerTab.PAIRING) },
                    icon = { Icon(Icons.Default.CloudQueue, contentDescription = "Сервер") },
                    label = { Text("Сервер") },
                    colors = navItemColors,
                    modifier = Modifier.testTag("nav_tab_pairing")
                )
                NavigationBarItem(
                    selected = selectedTab == ButlerTab.FILTERS,
                    onClick = { viewModel.selectTab(ButlerTab.FILTERS) },
                    icon = { Icon(Icons.Default.Apps, contentDescription = "Приложения") },
                    label = { Text("Приложения") },
                    colors = navItemColors,
                    modifier = Modifier.testTag("nav_tab_filters")
                )
                NavigationBarItem(
                    selected = selectedTab == ButlerTab.QUEUE,
                    onClick = { viewModel.selectTab(ButlerTab.QUEUE) },
                    icon = {
                        BadgedBox(
                            badge = {
                                if (queueMetrics.pendingCount > 0) {
                                    Badge { Text(queueMetrics.pendingCount.toString()) }
                                }
                            }
                        ) {
                            Icon(Icons.Default.Inbox, contentDescription = "Очередь")
                        }
                    },
                    label = { Text("Очередь") },
                    colors = navItemColors,
                    modifier = Modifier.testTag("nav_tab_queue")
                )
            }
        }
    ) { innerPadding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(innerPadding)
        ) {
            AnimatedVisibility(visible = !bannerMessage.isNullOrBlank()) {
                Surface(
                    color = MaterialTheme.colorScheme.surface,
                    shape = RoundedCornerShape(12.dp),
                    shadowElevation = 2.dp,
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 16.dp, vertical = 4.dp)
                        .clickable { viewModel.dismissBanner() }
                        .testTag("status_feedback_banner")
                ) {
                    Row(
                        modifier = Modifier.padding(horizontal = 14.dp, vertical = 10.dp),
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Text(
                            text = bannerMessage.orEmpty(),
                            style = MaterialTheme.typography.bodySmall.copy(fontSize = 13.sp),
                            color = MaterialTheme.colorScheme.onSurface
                        )
                    }
                }
            }

            Box(modifier = Modifier.weight(1f)) {
                when (selectedTab) {
                    ButlerTab.DASHBOARD -> DashboardScreen(
                        deviceConfig = deviceConfig,
                        environmentState = environmentState,
                        queueMetrics = queueMetrics,
                        hasNotificationAccess = hasNotificationAccess,
                        isListenerConnected = isListenerConnected,
                        isSyncingNow = isSyncingNow,
                        onOpenNotificationSettings = {
                            onOpenNotificationListenerSettings()
                            viewModel.refreshSystemPermissionAndNetworkState()
                        },
                        onOpenBatterySettings = onOpenBatteryOptimizationSettings,
                        onToggleSyncPaused = viewModel::setSyncPaused,
                        onToggleContentTransmission = viewModel::setContentTransmissionEnabled,
                        onPingServer = viewModel::triggerPingTest,
                        onSyncNow = viewModel::triggerManualBatchSync,
                        onSendSyntheticTest = viewModel::sendSyntheticTestEvent,
                        onSendUpdateRemoveLifecycleTest = viewModel::triggerSyntheticUpdateAndRemoveLifecycle,
                        onSendRealAndroidNotification = {
                            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
                                ContextCompat.checkSelfPermission(
                                    context,
                                    Manifest.permission.POST_NOTIFICATIONS
                                ) != PackageManager.PERMISSION_GRANTED
                            ) {
                                postNotificationPermissionLauncher.launch(Manifest.permission.POST_NOTIFICATIONS)
                            } else {
                                viewModel.postRealSystemTestNotification()
                            }
                        },
                        onSelectLoopbackMode = viewModel::setLoopbackSimulationMode,
                        onNavigateToPairing = { viewModel.selectTab(ButlerTab.PAIRING) }
                    )

                    ButlerTab.PAIRING -> PairingScreen(
                        deviceConfig = deviceConfig,
                        isSyncingNow = isSyncingNow,
                        onUpdateServerAddress = viewModel::updateServerAddressOnly,
                        onPairWithCodeOrQr = viewModel::pairWithCodeOrQr,
                        onRequestPhonePairingCode = viewModel::requestPairingCodeOnPhone,
                        onPollWebUiApproval = viewModel::pollWebUiPairingApproval,
                        onDisconnectDevice = viewModel::disconnectDevice
                    )

                    ButlerTab.FILTERS -> AppFiltersScreen(
                        deviceConfig = deviceConfig,
                        appRules = appFilterRules,
                        queueEvents = recentQueueEvents,
                        onDecryptEventPayload = viewModel::decryptEventPayload,
                        onChangeFilterMode = viewModel::setAppFilterMode,
                        onToggleContentTransmission = viewModel::setContentTransmissionEnabled,
                        onToggleMaskOtp = viewModel::setMaskOtpAndBankCodes,
                        onTogglePackageEnabled = viewModel::togglePackageFilter,
                        onAddCustomPackage = viewModel::addCustomPackageRule
                    )

                    ButlerTab.QUEUE -> QueueAndDiagnosticsScreen(
                        deviceConfig = deviceConfig,
                        queueMetrics = queueMetrics,
                        queueEvents = recentQueueEvents,
                        diagnosticLogs = diagnosticLogs,
                        onDecryptEventPayload = viewModel::decryptEventPayload,
                        onClearDelivered = viewModel::clearDeliveredQueueItems,
                        onClearAllQueue = viewModel::clearAllQueueItems,
                        onClearDiagnostics = viewModel::clearDiagnosticLogs,
                        onUpdateQueueLimits = viewModel::updateQueueLimits
                    )
                }
            }
        }
    }
}
