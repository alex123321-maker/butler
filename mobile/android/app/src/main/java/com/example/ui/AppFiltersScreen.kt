package com.example.ui

import androidx.activity.compose.BackHandler
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
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBackIos
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.ErrorOutline
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.Schedule
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.filled.Shield
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.example.data.local.AppFilterRuleEntity
import com.example.data.local.QueuedNotificationEntity
import com.example.data.repository.ButlerSyncRepository
import com.example.model.AppFilterMode
import com.example.model.NotificationPayloadDto
import com.example.security.DeviceConfigurationState
import com.example.ui.components.IosDivider
import com.example.ui.components.IosGroupedCard
import com.example.ui.components.IosSectionHeader
import com.example.ui.components.IosSegmentedControl
import com.example.ui.components.IosSettingsRow
import com.example.ui.components.IosSwitch
import com.example.ui.theme.IosGreen
import com.example.ui.theme.IosOrange
import com.example.ui.theme.IosPurple
import com.example.ui.theme.IosRed
import com.example.ui.theme.TelegramBlue
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

@Composable
fun AppFiltersScreen(
    deviceConfig: DeviceConfigurationState,
    appRules: List<AppFilterRuleEntity>,
    queueEvents: List<QueuedNotificationEntity>,
    onDecryptEventPayload: (QueuedNotificationEntity) -> NotificationPayloadDto?,
    onChangeFilterMode: (AppFilterMode) -> Unit,
    onToggleContentTransmission: (Boolean) -> Unit,
    onToggleMaskOtp: (Boolean) -> Unit,
    onTogglePackageEnabled: (String, Boolean) -> Unit,
    onAddCustomPackage: (String, String) -> Unit,
    modifier: Modifier = Modifier
) {
    val context = LocalContext.current
    var searchQuery by rememberSaveable { mutableStateOf("") }
    var customPkgInput by rememberSaveable { mutableStateOf("") }
    var customLabelInput by rememberSaveable { mutableStateOf("") }
    var showAddCustomRow by rememberSaveable { mutableStateOf(false) }
    var selectedPackageForHistory by rememberSaveable { mutableStateOf<String?>(null) }

    // Count notifications stored in queue per package
    val eventCountsByPackage = remember(queueEvents) {
        queueEvents.groupingBy { it.packageName }.eachCount()
    }

    // Resolve human-readable label via PackageManager if installed, or ButlerSyncRepository fallback
    val displayLabelsByPackage = remember(appRules, context) {
        val pm = context.packageManager
        appRules.associate { rule ->
            val pmLabel = runCatching {
                val info = pm.getApplicationInfo(rule.packageName, 0)
                pm.getApplicationLabel(info).toString()
            }.getOrNull()

            val bestLabel = when {
                !pmLabel.isNullOrBlank() && !pmLabel.equals("android", ignoreCase = true) -> pmLabel
                !rule.appLabel.equals("Android", ignoreCase = true) && rule.appLabel.isNotBlank() -> rule.appLabel
                else -> ButlerSyncRepository.inferAppLabel(rule.packageName)
            }
            rule.packageName to bestLabel
        }
    }

    // If an app is selected, show its full notification history screen
    val selectedRule = selectedPackageForHistory?.let { pkg ->
        appRules.find { it.packageName == pkg }
    }
    if (selectedPackageForHistory != null) {
        BackHandler {
            selectedPackageForHistory = null
        }
        val pkg = selectedPackageForHistory!!
        val appTitle = displayLabelsByPackage[pkg] ?: ButlerSyncRepository.inferAppLabel(pkg)
        val appEvents = remember(queueEvents, pkg) {
            queueEvents.filter { it.packageName == pkg }
        }
        val isEnabled = selectedRule?.let { rule ->
            when (deviceConfig.appFilterMode) {
                AppFilterMode.ALL_EXCEPT_EXCLUDED -> !rule.isExcludedInAllMode
                AppFilterMode.ONLY_INCLUDED -> rule.isIncludedInAllowlistMode
            }
        } ?: true

        AppNotificationHistoryDetailView(
            packageName = pkg,
            appTitle = appTitle,
            isSensitive = selectedRule?.isSensitiveSource == true,
            isEnabled = isEnabled,
            events = appEvents,
            onDecryptEventPayload = onDecryptEventPayload,
            onToggleEnabled = { checked -> onTogglePackageEnabled(pkg, checked) },
            onBack = { selectedPackageForHistory = null },
            modifier = modifier
        )
        return
    }

    val filteredRules = appRules.filter { rule ->
        val resolvedTitle = displayLabelsByPackage[rule.packageName] ?: rule.appLabel
        searchQuery.isBlank() ||
            resolvedTitle.contains(searchQuery, ignoreCase = true) ||
            rule.packageName.contains(searchQuery, ignoreCase = true)
    }

    Column(
        modifier = modifier
            .fillMaxSize()
            .padding(horizontal = 16.dp),
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .widthIn(max = 560.dp),
            verticalArrangement = Arrangement.spacedBy(4.dp)
        ) {
            item {
                Spacer(modifier = Modifier.height(6.dp))
                Surface(
                    modifier = Modifier.fillMaxWidth(),
                    shape = RoundedCornerShape(10.dp),
                    color = MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.7f)
                ) {
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 12.dp, vertical = 9.dp),
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Icon(
                            imageVector = Icons.Default.Search,
                            contentDescription = null,
                            tint = MaterialTheme.colorScheme.onSurfaceVariant,
                            modifier = Modifier.size(18.dp)
                        )
                        Spacer(modifier = Modifier.width(8.dp))
                        BasicTextField(
                            value = searchQuery,
                            onValueChange = { searchQuery = it },
                            singleLine = true,
                            textStyle = MaterialTheme.typography.bodyMedium.copy(
                                fontSize = 15.sp,
                                color = MaterialTheme.colorScheme.onSurface
                            ),
                            cursorBrush = SolidColor(TelegramBlue),
                            modifier = Modifier
                                .weight(1f)
                                .testTag("app_filter_search_input"),
                            decorationBox = { inner ->
                                if (searchQuery.isEmpty()) {
                                    Text(
                                        text = "Поиск по названию или пакету",
                                        style = MaterialTheme.typography.bodyMedium.copy(fontSize = 15.sp),
                                        color = MaterialTheme.colorScheme.onSurfaceVariant
                                    )
                                }
                                inner()
                            }
                        )
                    }
                }
            }

            // 1. Privacy & Filter Mode Section
            item {
                IosSectionHeader("Приватность")
                Text(
                    text = "Butler может самостоятельно использовать переданные сообщения для помощи: запоминать полезное, искать по архиву и скрывать шум. Приложения, к которым нужен запрет доступа, исключите здесь.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(bottom = 8.dp)
                )
                IosSegmentedControl(
                    options = listOf("Все кроме скрытых", "Белый список"),
                    selectedIndex = if (deviceConfig.appFilterMode == AppFilterMode.ALL_EXCEPT_EXCLUDED) 0 else 1,
                    onSelect = { idx ->
                        onChangeFilterMode(
                            if (idx == 0) AppFilterMode.ALL_EXCEPT_EXCLUDED else AppFilterMode.ONLY_INCLUDED
                        )
                    },
                    modifier = Modifier
                        .padding(bottom = 8.dp)
                        .testTag("filter_mode_card")
                )

                IosGroupedCard {
                    IosSettingsRow(
                        title = "Передавать текст",
                        icon = Icons.Default.Lock,
                        iconBackgroundColor = TelegramBlue,
                        trailing = {
                            IosSwitch(
                                checked = deviceConfig.isContentTransmissionEnabled,
                                onCheckedChange = onToggleContentTransmission,
                                modifier = Modifier.testTag("filters_content_transmission_switch")
                            )
                        }
                    )
                    IosDivider()

                    IosSettingsRow(
                        title = "Скрывать коды из банков и СМС",
                        icon = Icons.Default.Shield,
                        iconBackgroundColor = IosGreen,
                        trailing = {
                            IosSwitch(
                                checked = deviceConfig.maskOtpAndBankCodes,
                                onCheckedChange = onToggleMaskOtp,
                                modifier = Modifier.testTag("mask_otp_switch")
                            )
                        }
                    )
                }
            }

            // 2. Applications List (Tap any row to inspect its notifications)
            item {
                IosSectionHeader("Приложения (нажмите для просмотра уведомлений)")
                IosGroupedCard {
                    filteredRules.forEachIndexed { index, rule ->
                        val isEnabled = when (deviceConfig.appFilterMode) {
                            AppFilterMode.ALL_EXCEPT_EXCLUDED -> !rule.isExcludedInAllMode
                            AppFilterMode.ONLY_INCLUDED -> rule.isIncludedInAllowlistMode
                        }
                        val displayTitle = displayLabelsByPackage[rule.packageName] ?: rule.appLabel
                        val storedCount = eventCountsByPackage[rule.packageName] ?: 0
                        val totalCount = maxOf(storedCount, rule.totalObservedCount)

                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clickable { selectedPackageForHistory = rule.packageName }
                                .padding(horizontal = 16.dp, vertical = 10.dp)
                                .testTag("app_rule_card_${rule.packageName}"),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.SpaceBetween
                        ) {
                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                modifier = Modifier.weight(1f)
                            ) {
                                AppLetterSquircle(
                                    label = displayTitle,
                                    packageName = rule.packageName,
                                    isSensitive = rule.isSensitiveSource
                                )
                                Spacer(modifier = Modifier.width(12.dp))
                                Column(modifier = Modifier.weight(1f)) {
                                    Row(
                                        verticalAlignment = Alignment.CenterVertically,
                                        horizontalArrangement = Arrangement.spacedBy(6.dp)
                                    ) {
                                        Text(
                                            text = displayTitle,
                                            style = MaterialTheme.typography.bodyLarge.copy(
                                                fontSize = 16.sp,
                                                fontWeight = FontWeight.Medium
                                            ),
                                            color = MaterialTheme.colorScheme.onSurface,
                                            maxLines = 1,
                                            overflow = TextOverflow.Ellipsis
                                        )
                                        if (totalCount > 0) {
                                            Surface(
                                                color = TelegramBlue.copy(alpha = 0.14f),
                                                shape = RoundedCornerShape(6.dp)
                                            ) {
                                                Text(
                                                    text = totalCount.toString(),
                                                    style = MaterialTheme.typography.labelSmall.copy(
                                                        fontSize = 11.sp,
                                                        fontWeight = FontWeight.SemiBold
                                                    ),
                                                    color = TelegramBlue,
                                                    modifier = Modifier.padding(horizontal = 6.dp, vertical = 1.dp)
                                                )
                                            }
                                        }
                                    }
                                    Text(
                                        text = rule.packageName,
                                        style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp),
                                        color = if (rule.isSensitiveSource) IosOrange else MaterialTheme.colorScheme.onSurfaceVariant,
                                        maxLines = 1,
                                        overflow = TextOverflow.Ellipsis
                                    )
                                }
                            }

                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(6.dp)
                            ) {
                                IosSwitch(
                                    checked = isEnabled,
                                    onCheckedChange = { checked ->
                                        onTogglePackageEnabled(rule.packageName, checked)
                                    },
                                    modifier = Modifier.testTag("switch_pkg_${rule.packageName}")
                                )
                                Icon(
                                    imageVector = Icons.AutoMirrored.Filled.KeyboardArrowRight,
                                    contentDescription = "Открыть уведомления приложения",
                                    tint = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.45f),
                                    modifier = Modifier.size(18.dp)
                                )
                            }
                        }

                        if (index < filteredRules.lastIndex) {
                            IosDivider()
                        }
                    }

                    IosDivider(indented = false)

                    IosSettingsRow(
                        title = if (showAddCustomRow) "Отмена" else "Добавить приложение...",
                        icon = Icons.Default.Add,
                        iconBackgroundColor = TelegramBlue,
                        titleColor = TelegramBlue,
                        onClick = { showAddCustomRow = !showAddCustomRow },
                        modifier = Modifier.testTag("toggle_add_custom_package_button")
                    )

                    AnimatedVisibility(visible = showAddCustomRow) {
                        Column(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(horizontal = 16.dp, vertical = 10.dp),
                            verticalArrangement = Arrangement.spacedBy(10.dp)
                        ) {
                            BasicTextField(
                                value = customLabelInput,
                                onValueChange = { customLabelInput = it },
                                singleLine = true,
                                textStyle = MaterialTheme.typography.bodyLarge.copy(
                                    color = MaterialTheme.colorScheme.onSurface
                                ),
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .testTag("custom_package_label_input"),
                                decorationBox = { inner ->
                                    if (customLabelInput.isEmpty()) {
                                        Text("Название (напр. Slack)", color = MaterialTheme.colorScheme.onSurfaceVariant)
                                    }
                                    inner()
                                }
                            )
                            IosDivider(indented = false)
                            BasicTextField(
                                value = customPkgInput,
                                onValueChange = { customPkgInput = it },
                                singleLine = true,
                                textStyle = MaterialTheme.typography.bodyLarge.copy(
                                    color = MaterialTheme.colorScheme.primary
                                ),
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .testTag("custom_package_name_input"),
                                decorationBox = { inner ->
                                    if (customPkgInput.isEmpty()) {
                                        Text("Пакет (напр. com.Slack)", color = MaterialTheme.colorScheme.onSurfaceVariant)
                                    }
                                    inner()
                                }
                            )
                            IosDivider(indented = false)
                            IosSettingsRow(
                                title = "Сохранить",
                                titleColor = TelegramBlue,
                                onClick = {
                                    if (customPkgInput.isNotBlank()) {
                                        onAddCustomPackage(customPkgInput, customLabelInput)
                                        customPkgInput = ""
                                        customLabelInput = ""
                                        showAddCustomRow = false
                                    }
                                },
                                modifier = Modifier.testTag("save_custom_package_button")
                            )
                        }
                    }
                }
            }

            item { Spacer(modifier = Modifier.height(24.dp)) }
        }
    }
}

@Composable
private fun AppNotificationHistoryDetailView(
    packageName: String,
    appTitle: String,
    isSensitive: Boolean,
    isEnabled: Boolean,
    events: List<QueuedNotificationEntity>,
    onDecryptEventPayload: (QueuedNotificationEntity) -> NotificationPayloadDto?,
    onToggleEnabled: (Boolean) -> Unit,
    onBack: () -> Unit,
    modifier: Modifier = Modifier
) {
    val dateTimeFormatter = remember { SimpleDateFormat("dd.MM HH:mm:ss", Locale.getDefault()) }

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
            // iOS Back Navigation Row
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .clickable(onClick = onBack)
                    .padding(vertical = 10.dp)
                    .testTag("back_to_apps_list_button"),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Icon(
                    imageVector = Icons.AutoMirrored.Filled.ArrowBackIos,
                    contentDescription = "Назад",
                    tint = TelegramBlue,
                    modifier = Modifier.size(18.dp)
                )
                Text(
                    text = "Приложения",
                    style = MaterialTheme.typography.bodyLarge.copy(
                        fontSize = 16.sp,
                        fontWeight = FontWeight.Medium
                    ),
                    color = TelegramBlue
                )
            }

            // App Summary & Toggle Header Card
            IosGroupedCard {
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(16.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        modifier = Modifier.weight(1f)
                    ) {
                        AppLetterSquircle(
                            label = appTitle,
                            packageName = packageName,
                            isSensitive = isSensitive
                        )
                        Spacer(modifier = Modifier.width(12.dp))
                        Column(modifier = Modifier.weight(1f)) {
                            Text(
                                text = appTitle,
                                style = MaterialTheme.typography.titleMedium.copy(
                                    fontSize = 17.sp,
                                    fontWeight = FontWeight.SemiBold
                                )
                            )
                            Text(
                                text = packageName,
                                style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp),
                                color = MaterialTheme.colorScheme.onSurfaceVariant
                            )
                        }
                    }

                    IosSwitch(
                        checked = isEnabled,
                        onCheckedChange = onToggleEnabled
                    )
                }
            }

            IosSectionHeader("Уведомления приложения (${events.size})")

            if (events.isEmpty()) {
                IosGroupedCard {
                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(vertical = 36.dp, horizontal = 16.dp),
                        horizontalAlignment = Alignment.CenterHorizontally,
                        verticalArrangement = Arrangement.spacedBy(6.dp)
                    ) {
                        Text(
                            text = "Нет сохранённых уведомлений",
                            style = MaterialTheme.typography.bodyLarge,
                            color = MaterialTheme.colorScheme.onSurfaceVariant
                        )
                        Text(
                            text = "От этого пакета ($packageName) пока не поступало уведомлений в очередь.",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant
                        )
                    }
                }
            } else {
                LazyColumn(
                    modifier = Modifier.fillMaxSize(),
                    verticalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    items(events, key = { it.deviceEventId }) { entity ->
                        val decrypted = remember(entity.deviceEventId) {
                            onDecryptEventPayload(entity)
                        }
                        val (statusText, statusColor, statusIcon) = when (entity.deliveryState) {
                            "DELIVERED" -> Triple("Доставлено", IosGreen, Icons.Default.Check)
                            "CONFLICT_ERROR", "PERMANENT_ERROR" -> Triple("Ошибка", IosRed, Icons.Default.ErrorOutline)
                            else -> Triple("В очереди", TelegramBlue, Icons.Default.Schedule)
                        }
                        val changeText = when (entity.changeType) {
                            "UPDATED" -> "Обновлено"
                            "REMOVED" -> "Удалено"
                            else -> "Новое"
                        }

                        IosGroupedCard {
                            Column(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .padding(14.dp),
                                verticalArrangement = Arrangement.spacedBy(6.dp)
                            ) {
                                Row(
                                    modifier = Modifier.fillMaxWidth(),
                                    horizontalArrangement = Arrangement.SpaceBetween,
                                    verticalAlignment = Alignment.CenterVertically
                                ) {
                                    Row(
                                        verticalAlignment = Alignment.CenterVertically,
                                        horizontalArrangement = Arrangement.spacedBy(6.dp)
                                    ) {
                                        Box(
                                            modifier = Modifier
                                                .size(20.dp)
                                                .clip(CircleShape)
                                                .background(statusColor.copy(alpha = 0.16f)),
                                            contentAlignment = Alignment.Center
                                        ) {
                                            Icon(
                                                imageVector = statusIcon,
                                                contentDescription = null,
                                                tint = statusColor,
                                                modifier = Modifier.size(12.dp)
                                            )
                                        }
                                        Text(
                                            text = "$changeText • $statusText",
                                            style = MaterialTheme.typography.labelMedium.copy(fontSize = 12.sp),
                                            color = statusColor
                                        )
                                    }

                                    Text(
                                        text = dateTimeFormatter.format(Date(entity.observedAtEpochMs)),
                                        style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp),
                                        color = MaterialTheme.colorScheme.onSurfaceVariant
                                    )
                                }

                                if (entity.changeType == "REMOVED") {
                                    Text(
                                        text = "Уведомление смахнуто или закрыто системой",
                                        style = MaterialTheme.typography.bodyMedium,
                                        color = MaterialTheme.colorScheme.onSurfaceVariant
                                    )
                                } else if (entity.contentOmittedByMode) {
                                    Text(
                                        text = "Текст не сохранялся (режим «Только метаданные»)",
                                        style = MaterialTheme.typography.bodyMedium,
                                        color = MaterialTheme.colorScheme.onSurfaceVariant
                                    )
                                } else {
                                    if (!decrypted?.title.isNullOrBlank()) {
                                        Text(
                                            text = decrypted?.title.orEmpty(),
                                            style = MaterialTheme.typography.bodyLarge.copy(
                                                fontSize = 15.sp,
                                                fontWeight = FontWeight.SemiBold
                                            ),
                                            color = MaterialTheme.colorScheme.onSurface
                                        )
                                    }
                                    val bodyText = decrypted?.bigText?.takeIf { it.isNotBlank() }
                                        ?: decrypted?.text
                                        ?: decrypted?.subText
                                    if (!bodyText.isNullOrBlank()) {
                                        Text(
                                            text = bodyText,
                                            style = MaterialTheme.typography.bodyMedium.copy(fontSize = 14.sp),
                                            color = MaterialTheme.colorScheme.onSurface.copy(alpha = 0.88f)
                                        )
                                    }
                                    if (!decrypted?.messages.isNullOrEmpty()) {
                                        Column(
                                            modifier = Modifier
                                                .fillMaxWidth()
                                                .padding(top = 4.dp),
                                            verticalArrangement = Arrangement.spacedBy(3.dp)
                                        ) {
                                            decrypted?.messages?.forEach { msg ->
                                                Text(
                                                    text = if (!msg.sender.isNullOrBlank()) "${msg.sender}: ${msg.text}" else msg.text,
                                                    style = MaterialTheme.typography.bodySmall.copy(fontSize = 13.sp),
                                                    color = MaterialTheme.colorScheme.onSurfaceVariant
                                                )
                                            }
                                        }
                                    }
                                    if (decrypted?.title.isNullOrBlank() && bodyText.isNullOrBlank() && decrypted?.messages.isNullOrEmpty()) {
                                        Text(
                                            text = "Без текстового содержимого (системная карточка)",
                                            style = MaterialTheme.typography.bodySmall,
                                            color = MaterialTheme.colorScheme.onSurfaceVariant
                                        )
                                    }
                                }
                            }
                        }
                    }

                    item { Spacer(modifier = Modifier.height(24.dp)) }
                }
            }
        }
    }
}

@Composable
private fun AppLetterSquircle(
    label: String,
    packageName: String,
    isSensitive: Boolean
) {
    val bg = when {
        isSensitive -> IosOrange
        packageName.contains("telegram") -> TelegramBlue
        packageName.contains("whatsapp") -> IosGreen
        packageName.contains("gm") || packageName.contains("youtube") -> IosRed
        packageName.startsWith("com.android") || packageName == "android" -> IosPurple
        else -> TelegramBlue
    }
    val letter = label.firstOrNull { it.isLetterOrDigit() }?.uppercase() ?: "A"
    Box(
        modifier = Modifier
            .size(34.dp)
            .clip(RoundedCornerShape(8.dp))
            .background(bg),
        contentAlignment = Alignment.Center
    ) {
        Text(
            text = letter,
            style = MaterialTheme.typography.labelLarge.copy(
                fontSize = 15.sp,
                fontWeight = FontWeight.Bold
            ),
            color = Color.White
        )
    }
}
