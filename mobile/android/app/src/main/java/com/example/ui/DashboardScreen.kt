package com.example.ui

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.foundation.background
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.BatteryChargingFull
import androidx.compose.material.icons.filled.BugReport
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.CloudSync
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.Notifications
import androidx.compose.material.icons.filled.Pause
import androidx.compose.material.icons.filled.PriorityHigh
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Send
import androidx.compose.material.icons.filled.Sync
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.example.model.LoopbackSimulationMode
import com.example.model.RouteClassification
import com.example.security.DeviceConfigurationState
import com.example.security.TransportPolicyValidator
import com.example.service.SystemEnvironmentSnapshot
import com.example.ui.components.IosDivider
import com.example.ui.components.IosGroupedCard
import com.example.ui.components.IosSectionHeader
import com.example.ui.components.IosSettingsRow
import com.example.ui.components.IosSwitch
import com.example.ui.theme.IosGreen
import com.example.ui.theme.IosOrange
import com.example.ui.theme.IosPurple
import com.example.ui.theme.IosRed
import com.example.ui.theme.IosTeal
import com.example.ui.theme.TelegramBlue
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

@Composable
fun DashboardScreen(
    deviceConfig: DeviceConfigurationState,
    environmentState: SystemEnvironmentSnapshot,
    queueMetrics: QueueMetricsSummary,
    hasNotificationAccess: Boolean,
    isListenerConnected: Boolean,
    isSyncingNow: Boolean,
    onOpenNotificationSettings: () -> Unit,
    onOpenBatterySettings: () -> Unit,
    onToggleSyncPaused: (Boolean) -> Unit,
    onToggleContentTransmission: (Boolean) -> Unit,
    onPingServer: () -> Unit,
    onSyncNow: () -> Unit,
    onSendSyntheticTest: (Boolean) -> Unit,
    onSendUpdateRemoveLifecycleTest: () -> Unit,
    onSendRealAndroidNotification: () -> Unit,
    onSelectLoopbackMode: (LoopbackSimulationMode) -> Unit,
    onNavigateToPairing: () -> Unit,
    modifier: Modifier = Modifier
) {
    var showDeveloperTools by rememberSaveable { mutableStateOf(false) }

    val transportValidation = TransportPolicyValidator.validate(
        rawUrl = deviceConfig.serverUrl,
        allowCleartextLocalHttp = deviceConfig.allowCleartextLocalHttp
    )
    val isLanUnreachable =
        environmentState.isPrivateLanUnreachableOnCurrentNetwork(deviceConfig.serverUrl)

    val (statusTitle, statusAccent, statusIcon) = when {
        deviceConfig.isTokenRevoked -> Triple("Токен отозван", IosRed, Icons.Default.PriorityHigh)
        !deviceConfig.isPaired -> Triple("Не подключено", IosOrange, Icons.Default.PriorityHigh)
        !hasNotificationAccess -> Triple("Нужен доступ", IosOrange, Icons.Default.PriorityHigh)
        deviceConfig.isSyncPaused -> Triple("На паузе", IosOrange, Icons.Default.Pause)
        isLanUnreachable -> Triple("Вне домашней сети", IosOrange, Icons.Default.PriorityHigh)
        else -> Triple("Сопряжено", IosGreen, Icons.Default.Check)
    }

    val lastSyncTime = deviceConfig.lastSuccessfulSyncEpochMs?.let {
        SimpleDateFormat("HH:mm", Locale.getDefault()).format(Date(it))
    } ?: "—"

    Column(
        modifier = modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 16.dp, vertical = 8.dp),
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .widthIn(max = 560.dp)
        ) {
            // 1. Minimalist iOS / Telegram Centered Hero Header
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(vertical = 18.dp)
                    .testTag("hero_status_card"),
                horizontalAlignment = Alignment.CenterHorizontally
            ) {
                Box(
                    modifier = Modifier
                        .size(68.dp)
                        .clip(CircleShape)
                        .background(statusAccent.copy(alpha = 0.15f)),
                    contentAlignment = Alignment.Center
                ) {
                    Box(
                        modifier = Modifier
                            .size(48.dp)
                            .clip(CircleShape)
                            .background(statusAccent),
                        contentAlignment = Alignment.Center
                    ) {
                        Icon(
                            imageVector = statusIcon,
                            contentDescription = statusTitle,
                            tint = Color.White,
                            modifier = Modifier.size(28.dp)
                        )
                    }
                }

                Spacer(modifier = Modifier.height(10.dp))

                Text(
                    text = statusTitle,
                    style = MaterialTheme.typography.headlineMedium.copy(
                        fontSize = 22.sp,
                        fontWeight = FontWeight.Bold
                    ),
                    color = MaterialTheme.colorScheme.onBackground
                )

                Text(
                    text = "${deviceConfig.serverName ?: "Сервер не выбран"} • Синхр. $lastSyncTime",
                    style = MaterialTheme.typography.bodyMedium.copy(fontSize = 14.sp),
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )

                if (!deviceConfig.lastSyncErrorText.isNullOrBlank()) {
                    Spacer(modifier = Modifier.height(6.dp))
                    Text(
                        text = deviceConfig.lastSyncErrorText,
                        style = MaterialTheme.typography.bodySmall,
                        color = IosRed
                    )
                }
            }

            // 2. Compact 3-Metric Pill Card
            IosGroupedCard(modifier = Modifier.testTag("queue_telemetry_card")) {
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(vertical = 14.dp),
                    horizontalArrangement = Arrangement.SpaceEvenly,
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    CompactStatItem(
                        value = queueMetrics.pendingCount.toString(),
                        label = "В очереди",
                        color = TelegramBlue
                    )
                    CompactStatItem(
                        value = queueMetrics.deliveredCount.toString(),
                        label = "Доставлено",
                        color = IosGreen
                    )
                    CompactStatItem(
                        value = (queueMetrics.errorCount + deviceConfig.droppedOverflowCount).toString(),
                        label = "Ошибки",
                        color = if (queueMetrics.errorCount + deviceConfig.droppedOverflowCount > 0) IosRed else MaterialTheme.colorScheme.onSurfaceVariant
                    )
                }
            }

            // 3. Primary Controls Group
            IosSectionHeader("Управление")
            IosGroupedCard {
                IosSettingsRow(
                    title = "Синхронизация",
                    icon = Icons.Default.Sync,
                    iconBackgroundColor = IosGreen,
                    modifier = Modifier.testTag("toggle_sync_pause_button"),
                    trailing = {
                        IosSwitch(
                            checked = !deviceConfig.isSyncPaused,
                            onCheckedChange = { enabled -> onToggleSyncPaused(!enabled) }
                        )
                    }
                )
                IosDivider()

                IosSettingsRow(
                    title = "Доступ к уведомлениям",
                    icon = Icons.Default.Notifications,
                    iconBackgroundColor = IosRed,
                    valueText = if (hasNotificationAccess || isListenerConnected) "Вкл." else "Выкл.",
                    valueColor = if (hasNotificationAccess || isListenerConnected) IosGreen else IosOrange,
                    showChevron = true,
                    onClick = onOpenNotificationSettings,
                    modifier = Modifier.testTag("open_notification_access_settings_button")
                )
                IosDivider()

                IosSettingsRow(
                    title = "Передавать текст",
                    icon = Icons.Default.Lock,
                    iconBackgroundColor = TelegramBlue,
                    modifier = Modifier.testTag("quick_privacy_card"),
                    trailing = {
                        IosSwitch(
                            checked = deviceConfig.isContentTransmissionEnabled,
                            onCheckedChange = onToggleContentTransmission,
                            modifier = Modifier.testTag("quick_content_transmission_switch")
                        )
                    }
                )
                IosDivider()

                IosSettingsRow(
                    title = "Сервер Butler",
                    icon = Icons.Default.CloudSync,
                    iconBackgroundColor = IosPurple,
                    valueText = if (deviceConfig.isPaired) "Настроен" else "Подключить",
                    showChevron = true,
                    onClick = onNavigateToPairing,
                    modifier = Modifier.testTag("open_pairing_from_hero_button")
                )
            }

            // 4. Quick Actions Group
            IosSectionHeader("Быстрые действия")
            IosGroupedCard(modifier = Modifier.testTag("verification_actions_card")) {
                IosSettingsRow(
                    title = "Отправить тестовое событие",
                    icon = Icons.Default.Send,
                    iconBackgroundColor = TelegramBlue,
                    showChevron = true,
                    onClick = { if (!isSyncingNow) onSendSyntheticTest(false) },
                    modifier = Modifier.testTag("send_test_event_button")
                )
                IosDivider()

                IosSettingsRow(
                    title = "Синхронизировать сейчас",
                    icon = Icons.Default.CloudSync,
                    iconBackgroundColor = IosGreen,
                    valueText = if (isSyncingNow) "..." else null,
                    showChevron = true,
                    onClick = { if (!isSyncingNow) onSyncNow() },
                    modifier = Modifier.testTag("sync_now_button")
                )
                IosDivider()

                IosSettingsRow(
                    title = "Проверить связь",
                    icon = Icons.Default.Refresh,
                    iconBackgroundColor = IosTeal,
                    showChevron = true,
                    onClick = { if (!isSyncingNow) onPingServer() },
                    modifier = Modifier.testTag("ping_server_button")
                )
            }

            // 5. Optional Collapsible Developer / Verification Tools
            IosSectionHeader("Дополнительно")
            IosGroupedCard {
                IosSettingsRow(
                    title = "Фоновый режим батареи",
                    icon = Icons.Default.BatteryChargingFull,
                    iconBackgroundColor = IosGreen,
                    valueText = environmentState.transport.labelRu,
                    showChevron = true,
                    onClick = onOpenBatterySettings,
                    modifier = Modifier.testTag("open_battery_settings_button")
                )
                IosDivider()

                IosSettingsRow(
                    title = "Инструменты проверки",
                    icon = Icons.Default.BugReport,
                    iconBackgroundColor = IosOrange,
                    valueText = if (showDeveloperTools) "Скрыть" else "Открыть",
                    showChevron = true,
                    onClick = { showDeveloperTools = !showDeveloperTools },
                    modifier = Modifier.testTag("toggle_dev_tools_row")
                )

                AnimatedVisibility(visible = showDeveloperTools) {
                    Column {
                        IosDivider()
                        IosSettingsRow(
                            title = "Тест маскировки OTP-кода",
                            titleColor = TelegramBlue,
                            onClick = { onSendSyntheticTest(true) },
                            modifier = Modifier.testTag("send_otp_test_event_button")
                        )
                        IosDivider(indented = false)
                        IosSettingsRow(
                            title = "Тест обновления и удаления",
                            titleColor = TelegramBlue,
                            onClick = onSendUpdateRemoveLifecycleTest,
                            modifier = Modifier.testTag("send_lifecycle_test_button")
                        )
                        IosDivider(indented = false)
                        IosSettingsRow(
                            title = "Системное уведомление в шторку",
                            titleColor = TelegramBlue,
                            onClick = onSendRealAndroidNotification,
                            modifier = Modifier.testTag("post_real_android_notification_button")
                        )

                        if (transportValidation.classification == RouteClassification.LOOPBACK_SANDBOX) {
                            IosDivider(indented = false)
                            Column(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .padding(horizontal = 16.dp, vertical = 10.dp)
                                    .testTag("loopback_simulator_card")
                            ) {
                                Text(
                                    text = "Сценарий ответа стенда:",
                                    style = MaterialTheme.typography.bodySmall,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant
                                )
                                Spacer(modifier = Modifier.height(6.dp))
                                Row(
                                    modifier = Modifier
                                        .fillMaxWidth()
                                        .horizontalScroll(rememberScrollState()),
                                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                                ) {
                                    LoopbackSimulationMode.entries.forEach { mode ->
                                        FilterChip(
                                            selected = deviceConfig.loopbackSimulationMode == mode,
                                            onClick = { onSelectLoopbackMode(mode) },
                                            label = { Text(mode.labelRu) },
                                            modifier = Modifier.testTag("loopback_mode_${mode.name.lowercase()}")
                                        )
                                    }
                                }
                            }
                        }
                    }
                }
            }

            Spacer(modifier = Modifier.height(24.dp))
        }
    }
}

@Composable
private fun CompactStatItem(
    value: String,
    label: String,
    color: Color
) {
    Column(horizontalAlignment = Alignment.CenterHorizontally) {
        Text(
            text = value,
            style = MaterialTheme.typography.titleLarge.copy(
                fontSize = 22.sp,
                fontWeight = FontWeight.Bold
            ),
            color = color
        )
        Text(
            text = label,
            style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp),
            color = MaterialTheme.colorScheme.onSurfaceVariant
        )
    }
}
