package com.example.ui

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
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
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.DeleteOutline
import androidx.compose.material.icons.filled.ErrorOutline
import androidx.compose.material.icons.filled.Schedule
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.example.data.local.DiagnosticLogEntity
import com.example.data.local.QueuedNotificationEntity
import com.example.model.NotificationPayloadDto
import com.example.security.DeviceConfigurationState
import com.example.ui.components.IosDivider
import com.example.ui.components.IosGroupedCard
import com.example.ui.components.IosSectionHeader
import com.example.ui.components.IosSegmentedControl
import com.example.ui.components.IosSettingsRow
import com.example.ui.theme.IosGreen
import com.example.ui.theme.IosOrange
import com.example.ui.theme.IosRed
import com.example.ui.theme.TelegramBlue
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

@Composable
fun QueueAndDiagnosticsScreen(
    deviceConfig: DeviceConfigurationState,
    queueMetrics: QueueMetricsSummary,
    queueEvents: List<QueuedNotificationEntity>,
    diagnosticLogs: List<DiagnosticLogEntity>,
    onDecryptEventPayload: (QueuedNotificationEntity) -> NotificationPayloadDto?,
    onClearDelivered: () -> Unit,
    onClearAllQueue: () -> Unit,
    onClearDiagnostics: () -> Unit,
    onUpdateQueueLimits: (Int, Int, Int) -> Unit,
    modifier: Modifier = Modifier
) {
    var selectedSection by rememberSaveable { mutableIntStateOf(0) }
    val revealedEventIds = remember { mutableStateMapOf<String, Boolean>() }
    val timeFormatter = remember { SimpleDateFormat("HH:mm", Locale.getDefault()) }

    Column(
        modifier = modifier
            .fillMaxSize()
            .padding(horizontal = 16.dp),
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .widthIn(max = 560.dp)
        ) {
            Spacer(modifier = Modifier.height(8.dp))

            IosSegmentedControl(
                options = listOf("Уведомления (${queueEvents.size})", "Журнал (${diagnosticLogs.size})"),
                selectedIndex = selectedSection,
                onSelect = { selectedSection = it }
            )

            Spacer(modifier = Modifier.height(8.dp))

            if (selectedSection == 0) {
                LazyColumn(
                    modifier = Modifier.fillMaxSize(),
                    verticalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    if (queueEvents.isEmpty()) {
                        item {
                            IosGroupedCard(modifier = Modifier.testTag("empty_queue_card")) {
                                Column(
                                    modifier = Modifier
                                        .fillMaxWidth()
                                        .padding(vertical = 32.dp),
                                    horizontalAlignment = Alignment.CenterHorizontally
                                ) {
                                    Text(
                                        text = "Очередь пуста",
                                        style = MaterialTheme.typography.titleMedium,
                                        color = MaterialTheme.colorScheme.onSurfaceVariant
                                    )
                                }
                            }
                        }
                    } else {
                        item {
                            IosGroupedCard {
                                queueEvents.forEachIndexed { index, entity ->
                                    val isRevealed = revealedEventIds[entity.deviceEventId] == true
                                    val (statusLabel, badgeColor, statusIcon) = when (entity.deliveryState) {
                                        "DELIVERED" -> Triple("Доставлено", IosGreen, Icons.Default.Check)
                                        "CONFLICT_ERROR", "PERMANENT_ERROR" -> Triple("Ошибка", IosRed, Icons.Default.ErrorOutline)
                                        else -> Triple("В очереди", TelegramBlue, Icons.Default.Schedule)
                                    }

                                    val changeLabel = when (entity.changeType) {
                                        "UPDATED" -> "Обновлено"
                                        "REMOVED" -> "Удалено"
                                        else -> "Новое"
                                    }

                                    Column(
                                        modifier = Modifier
                                            .fillMaxWidth()
                                            .clickable {
                                                if (!entity.contentOmittedByMode && entity.encryptedPayloadBlob != null) {
                                                    revealedEventIds[entity.deviceEventId] = !isRevealed
                                                }
                                            }
                                            .padding(horizontal = 16.dp, vertical = 12.dp)
                                            .testTag("queue_item_${entity.deviceEventId}")
                                    ) {
                                        Row(
                                            modifier = Modifier.fillMaxWidth(),
                                            verticalAlignment = Alignment.CenterVertically,
                                            horizontalArrangement = Arrangement.SpaceBetween
                                        ) {
                                            Row(
                                                verticalAlignment = Alignment.CenterVertically,
                                                modifier = Modifier.weight(1f)
                                            ) {
                                                Box(
                                                    modifier = Modifier
                                                        .size(28.dp)
                                                        .clip(CircleShape)
                                                        .background(badgeColor.copy(alpha = 0.16f)),
                                                    contentAlignment = Alignment.Center
                                                ) {
                                                    Icon(
                                                        imageVector = statusIcon,
                                                        contentDescription = null,
                                                        tint = badgeColor,
                                                        modifier = Modifier.size(16.dp)
                                                    )
                                                }
                                                Spacer(modifier = Modifier.width(12.dp))
                                                Column(modifier = Modifier.weight(1f)) {
                                                    Text(
                                                        text = formatCleanPackageTitle(entity.packageName),
                                                        style = MaterialTheme.typography.bodyLarge.copy(
                                                            fontSize = 16.sp,
                                                            fontWeight = FontWeight.Medium
                                                        ),
                                                        maxLines = 1,
                                                        overflow = TextOverflow.Ellipsis
                                                    )
                                                    Text(
                                                        text = buildString {
                                                            append("$statusLabel • $changeLabel")
                                                            if (entity.contentRedactedByUserRule) append(" • Код скрыт")
                                                            if (entity.contentOmittedByMode) append(" • Без текста")
                                                        },
                                                        style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp),
                                                        color = MaterialTheme.colorScheme.onSurfaceVariant
                                                    )
                                                }
                                            }

                                            Column(horizontalAlignment = Alignment.End) {
                                                Text(
                                                    text = timeFormatter.format(Date(entity.observedAtEpochMs)),
                                                    style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp),
                                                    color = MaterialTheme.colorScheme.onSurfaceVariant
                                                )
                                                if (!entity.contentOmittedByMode && entity.encryptedPayloadBlob != null) {
                                                    Text(
                                                        text = if (isRevealed) "Скрыть" else "Текст",
                                                        style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.sp),
                                                        color = TelegramBlue,
                                                        modifier = Modifier.testTag("reveal_payload_${entity.deviceEventId}")
                                                    )
                                                }
                                            }
                                        }

                                        AnimatedVisibility(visible = isRevealed) {
                                            val decrypted = remember(entity.deviceEventId) {
                                                onDecryptEventPayload(entity)
                                            }
                                            Surface(
                                                color = MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.55f),
                                                shape = RoundedCornerShape(10.dp),
                                                modifier = Modifier
                                                    .fillMaxWidth()
                                                    .padding(top = 8.dp)
                                            ) {
                                                Column(modifier = Modifier.padding(10.dp)) {
                                                    if (!decrypted?.title.isNullOrBlank()) {
                                                        Text(
                                                            text = decrypted?.title.orEmpty(),
                                                            style = MaterialTheme.typography.bodySmall,
                                                            fontWeight = FontWeight.SemiBold
                                                        )
                                                    }
                                                    Text(
                                                        text = decrypted?.text ?: "Нет текста",
                                                        style = MaterialTheme.typography.bodySmall,
                                                        color = MaterialTheme.colorScheme.onSurfaceVariant
                                                    )
                                                }
                                            }
                                        }
                                    }

                                    if (index < queueEvents.lastIndex) {
                                        IosDivider()
                                    }
                                }
                            }
                        }

                        item {
                            IosSectionHeader("Очистка")
                            IosGroupedCard {
                                IosSettingsRow(
                                    title = "Удалить доставленные",
                                    icon = Icons.Default.DeleteOutline,
                                    iconBackgroundColor = TelegramBlue,
                                    onClick = onClearDelivered,
                                    modifier = Modifier.testTag("clear_delivered_events_button")
                                )
                                IosDivider()
                                IosSettingsRow(
                                    title = "Очистить всю очередь",
                                    icon = Icons.Default.DeleteOutline,
                                    iconBackgroundColor = IosRed,
                                    titleColor = IosRed,
                                    onClick = onClearAllQueue,
                                    modifier = Modifier.testTag("clear_all_queue_button")
                                )
                            }
                        }
                    }

                    item { Spacer(modifier = Modifier.height(24.dp)) }
                }
            } else {
                LazyColumn(
                    modifier = Modifier.fillMaxSize(),
                    verticalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    item {
                        IosGroupedCard {
                            if (diagnosticLogs.isEmpty()) {
                                Box(
                                    modifier = Modifier
                                        .fillMaxWidth()
                                        .padding(vertical = 24.dp),
                                    contentAlignment = Alignment.Center
                                ) {
                                    Text(
                                        text = "Журнал событий пуст",
                                        style = MaterialTheme.typography.bodyMedium,
                                        color = MaterialTheme.colorScheme.onSurfaceVariant
                                    )
                                }
                            } else {
                                diagnosticLogs.forEachIndexed { index, log ->
                                    val dotColor = when (log.level) {
                                        "ERROR" -> IosRed
                                        "WARN" -> IosOrange
                                        else -> IosGreen
                                    }
                                    Row(
                                        modifier = Modifier
                                            .fillMaxWidth()
                                            .padding(horizontal = 16.dp, vertical = 10.dp),
                                        verticalAlignment = Alignment.CenterVertically,
                                        horizontalArrangement = Arrangement.SpaceBetween
                                    ) {
                                        Row(
                                            verticalAlignment = Alignment.CenterVertically,
                                            modifier = Modifier.weight(1f)
                                        ) {
                                            Box(
                                                modifier = Modifier
                                                    .size(8.dp)
                                                    .clip(CircleShape)
                                                    .background(dotColor)
                                            )
                                            Spacer(modifier = Modifier.width(10.dp))
                                            Text(
                                                text = log.summaryRu,
                                                style = MaterialTheme.typography.bodyMedium.copy(fontSize = 14.sp),
                                                color = MaterialTheme.colorScheme.onSurface,
                                                maxLines = 2,
                                                overflow = TextOverflow.Ellipsis
                                            )
                                        }
                                        Spacer(modifier = Modifier.width(8.dp))
                                        Text(
                                            text = timeFormatter.format(Date(log.timestampEpochMs)),
                                            style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp),
                                            color = MaterialTheme.colorScheme.onSurfaceVariant
                                        )
                                    }
                                    if (index < diagnosticLogs.lastIndex) {
                                        IosDivider(indented = false)
                                    }
                                }
                            }
                        }
                    }

                    if (diagnosticLogs.isNotEmpty()) {
                        item {
                            IosGroupedCard {
                                IosSettingsRow(
                                    title = "Очистить журнал",
                                    titleColor = IosRed,
                                    onClick = onClearDiagnostics,
                                    modifier = Modifier.testTag("clear_diagnostics_button")
                                )
                            }
                        }
                    }

                    item { Spacer(modifier = Modifier.height(24.dp)) }
                }
            }
        }
    }
}

private fun formatCleanPackageTitle(packageName: String): String {
    return com.example.data.repository.ButlerSyncRepository.inferAppLabel(packageName)
}
