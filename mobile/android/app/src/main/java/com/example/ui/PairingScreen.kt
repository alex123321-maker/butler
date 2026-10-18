package com.example.ui

import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Cloud
import androidx.compose.material.icons.filled.Key
import androidx.compose.material.icons.filled.LinkOff
import androidx.compose.material.icons.filled.LockOpen
import androidx.compose.material.icons.filled.Smartphone
import androidx.compose.material3.AssistChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.example.security.DeviceConfigurationState
import com.example.security.TransportPolicyValidator
import com.example.ui.components.IosDivider
import com.example.ui.components.IosGroupedCard
import com.example.ui.components.IosSectionFooter
import com.example.ui.components.IosSectionHeader
import com.example.ui.components.IosSegmentedControl
import com.example.ui.components.IosSettingsRow
import com.example.ui.components.IosSwitch
import com.example.ui.theme.IosGreen
import com.example.ui.theme.IosOrange
import com.example.ui.theme.IosPurple
import com.example.ui.theme.IosRed
import com.example.ui.theme.TelegramBlue

@Composable
fun PairingScreen(
    deviceConfig: DeviceConfigurationState,
    isSyncingNow: Boolean,
    onUpdateServerAddress: (String, Boolean) -> Unit,
    onPairWithCodeOrQr: (String, Boolean, String) -> Unit,
    onRequestPhonePairingCode: (String, Boolean) -> Unit,
    onPollWebUiApproval: () -> Unit,
    onDisconnectDevice: () -> Unit,
    modifier: Modifier = Modifier
) {
    var serverUrlInput by rememberSaveable(deviceConfig.serverUrl) {
        mutableStateOf(deviceConfig.serverUrl)
    }
    var allowCleartextInput by rememberSaveable(deviceConfig.allowCleartextLocalHttp) {
        mutableStateOf(deviceConfig.allowCleartextLocalHttp)
    }
    var selectedPairingMethod by rememberSaveable { mutableIntStateOf(0) }
    var codeOrQrInput by rememberSaveable { mutableStateOf("") }

    val validation = TransportPolicyValidator.validate(serverUrlInput, allowCleartextInput)

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
            // 1. Server Address Group
            IosSectionHeader("Адрес сервера")
            IosGroupedCard(modifier = Modifier.testTag("server_endpoint_card")) {
                BoxFieldRow(
                    label = "URL",
                    value = serverUrlInput,
                    placeholder = "https://butler.example.com",
                    onValueChange = { serverUrlInput = it },
                    testTag = "server_url_input"
                )
                IosDivider(indented = false)

                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .horizontalScroll(rememberScrollState())
                        .padding(horizontal = 12.dp, vertical = 8.dp),
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    AssistChip(
                        onClick = {
                            serverUrlInput = TransportPolicyValidator.LOOPBACK_BASE_URL
                            onUpdateServerAddress(serverUrlInput, allowCleartextInput)
                        },
                        label = { Text("Стенд") },
                        modifier = Modifier.testTag("preset_loopback_chip")
                    )
                    AssistChip(
                        onClick = {
                            serverUrlInput = "http://192.168.1.50:8080"
                            allowCleartextInput = true
                            onUpdateServerAddress(serverUrlInput, true)
                        },
                        label = { Text("Wi-Fi LAN") },
                        modifier = Modifier.testTag("preset_lan_chip")
                    )
                    AssistChip(
                        onClick = {
                            serverUrlInput = "http://100.101.102.103:8080"
                            allowCleartextInput = true
                            onUpdateServerAddress(serverUrlInput, true)
                        },
                        label = { Text("Tailscale") },
                        modifier = Modifier.testTag("preset_tailscale_chip")
                    )
                    AssistChip(
                        onClick = {
                            serverUrlInput = "https://butler.example.com"
                            onUpdateServerAddress(serverUrlInput, allowCleartextInput)
                        },
                        label = { Text("HTTPS") },
                        modifier = Modifier.testTag("preset_https_chip")
                    )
                }
                IosDivider(indented = false)

                IosSettingsRow(
                    title = "Локальный HTTP",
                    icon = Icons.Default.LockOpen,
                    iconBackgroundColor = IosOrange,
                    trailing = {
                        IosSwitch(
                            checked = allowCleartextInput,
                            onCheckedChange = {
                                allowCleartextInput = it
                                onUpdateServerAddress(serverUrlInput, it)
                            },
                            modifier = Modifier.testTag("allow_cleartext_switch")
                        )
                    }
                )
                IosDivider()

                IosSettingsRow(
                    title = "Сохранить адрес",
                    titleColor = if (validation.isValid) TelegramBlue else MaterialTheme.colorScheme.onSurfaceVariant,
                    valueText = if (validation.isValid) "Готово" else "Ошибка",
                    onClick = {
                        if (validation.isValid) {
                            onUpdateServerAddress(serverUrlInput, allowCleartextInput)
                        }
                    },
                    modifier = Modifier.testTag("save_server_url_button")
                )
            }
            if (!validation.isValid && !validation.warningOrErrorRu.isNullOrBlank()) {
                IosSectionFooter(validation.warningOrErrorRu)
            }

            // 2. Pairing Section
            IosSectionHeader("Сопряжение")
            IosSegmentedControl(
                options = listOf("По коду / QR", "Через Web UI"),
                selectedIndex = selectedPairingMethod,
                onSelect = { selectedPairingMethod = it },
                modifier = Modifier.padding(bottom = 10.dp)
            )

            IosGroupedCard(modifier = Modifier.testTag("pairing_flow_card")) {
                if (selectedPairingMethod == 0) {
                    BoxFieldRow(
                        label = "Код",
                        value = codeOrQrInput,
                        placeholder = "849-204 или butler://pair?...",
                        onValueChange = { codeOrQrInput = it },
                        testTag = "pairing_code_input"
                    )
                    IosDivider(indented = false)

                    IosSettingsRow(
                        title = "Подключить устройство",
                        icon = Icons.Default.Key,
                        iconBackgroundColor = TelegramBlue,
                        titleColor = TelegramBlue,
                        showChevron = true,
                        onClick = {
                            if (validation.isValid && codeOrQrInput.isNotBlank() && !isSyncingNow) {
                                onPairWithCodeOrQr(serverUrlInput, allowCleartextInput, codeOrQrInput)
                            }
                        },
                        modifier = Modifier.testTag("submit_pair_code_button")
                    )
                } else {
                    if (!deviceConfig.pendingVerificationCode.isNullOrBlank()) {
                        Column(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(vertical = 18.dp),
                            horizontalAlignment = Alignment.CenterHorizontally
                        ) {
                            Text(
                                text = deviceConfig.pendingVerificationCode,
                                style = MaterialTheme.typography.headlineLarge.copy(
                                    fontSize = 32.sp,
                                    fontWeight = FontWeight.Bold,
                                    letterSpacing = 2.sp
                                ),
                                color = TelegramBlue,
                                textAlign = TextAlign.Center
                            )
                            Text(
                                text = "Введите этот код в Web UI Butler",
                                style = MaterialTheme.typography.bodySmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant
                            )
                        }
                        IosDivider(indented = false)
                    }

                    IosSettingsRow(
                        title = "Получить код на телефон",
                        icon = Icons.Default.Smartphone,
                        iconBackgroundColor = TelegramBlue,
                        showChevron = true,
                        onClick = {
                            if (validation.isValid && !isSyncingNow) {
                                onRequestPhonePairingCode(serverUrlInput, allowCleartextInput)
                            }
                        },
                        modifier = Modifier.testTag("request_phone_code_button")
                    )
                    IosDivider()

                    IosSettingsRow(
                        title = "Проверить подтверждение",
                        icon = Icons.Default.Cloud,
                        iconBackgroundColor = IosGreen,
                        showChevron = true,
                        onClick = {
                            if (!deviceConfig.pendingPairingSessionId.isNullOrBlank() && !isSyncingNow) {
                                onPollWebUiApproval()
                            }
                        },
                        modifier = Modifier.testTag("poll_webui_approval_button")
                    )
                }
            }

            // 3. Device & Disconnect Section
            IosSectionHeader("Устройство")
            IosGroupedCard(modifier = Modifier.testTag("device_credential_card")) {
                IosSettingsRow(
                    title = "Устройство",
                    icon = Icons.Default.Smartphone,
                    iconBackgroundColor = IosPurple,
                    valueText = deviceConfig.deviceName
                )
                IosDivider()

                IosSettingsRow(
                    title = "Защита ключа",
                    icon = Icons.Default.Key,
                    iconBackgroundColor = IosGreen,
                    valueText = if (deviceConfig.isPaired) "Keystore AES-256" else "Нет ключа"
                )
                IosDivider()

                IosSettingsRow(
                    title = "Отключить устройство",
                    icon = Icons.Default.LinkOff,
                    iconBackgroundColor = IosRed,
                    titleColor = IosRed,
                    onClick = onDisconnectDevice,
                    modifier = Modifier.testTag("disconnect_device_button")
                )
            }

            IosSectionFooter("При отключении локальная очередь и ключ удаляются. Историю на сервере можно удалить отдельно в Web UI.")
            Spacer(modifier = Modifier.height(24.dp))
        }
    }
}

@Composable
private fun BoxFieldRow(
    label: String,
    value: String,
    placeholder: String,
    onValueChange: (String) -> Unit,
    testTag: String
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 14.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Text(
            text = label,
            style = MaterialTheme.typography.bodyLarge.copy(
                fontSize = 16.sp,
                fontWeight = FontWeight.Medium
            ),
            color = MaterialTheme.colorScheme.onSurface,
            modifier = Modifier.widthIn(min = 64.dp)
        )
        BasicTextField(
            value = value,
            onValueChange = onValueChange,
            singleLine = true,
            textStyle = MaterialTheme.typography.bodyLarge.copy(
                fontSize = 16.sp,
                color = MaterialTheme.colorScheme.primary
            ),
            cursorBrush = SolidColor(TelegramBlue),
            modifier = Modifier
                .weight(1f)
                .testTag(testTag),
            decorationBox = { innerTextField ->
                Surface(color = androidx.compose.ui.graphics.Color.Transparent) {
                    if (value.isEmpty()) {
                        Text(
                            text = placeholder,
                            style = MaterialTheme.typography.bodyLarge.copy(fontSize = 16.sp),
                            color = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.5f)
                        )
                    }
                    innerTextField()
                }
            }
        )
    }
}
