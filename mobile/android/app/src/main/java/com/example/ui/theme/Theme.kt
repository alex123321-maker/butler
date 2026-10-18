package com.example.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color

private val DarkColorScheme = darkColorScheme(
    primary = TelegramBlueDark,
    onPrimary = Color.White,
    primaryContainer = Color(0xFF132A4A),
    onPrimaryContainer = Color(0xFFD6E8FF),
    secondary = IosGreen,
    onSecondary = Color.White,
    secondaryContainer = Color(0xFF113821),
    onSecondaryContainer = Color(0xFFD1FAE5),
    tertiary = IosOrange,
    onTertiary = Color.White,
    tertiaryContainer = Color(0xFF3D2808),
    onTertiaryContainer = Color(0xFFFFE5B4),
    error = IosRed,
    onError = Color.White,
    errorContainer = Color(0xFF3E1414),
    onErrorContainer = Color(0xFFFFD6D6),
    background = IosDarkBackground,
    onBackground = IosDarkPrimaryText,
    surface = IosDarkSurface,
    onSurface = IosDarkPrimaryText,
    surfaceVariant = IosDarkSecondarySurface,
    onSurfaceVariant = IosDarkSecondaryText,
    outline = IosDarkDivider
)

private val LightColorScheme = lightColorScheme(
    primary = TelegramBlue,
    onPrimary = Color.White,
    primaryContainer = Color(0xFFE5F1FF),
    onPrimaryContainer = Color(0xFF004085),
    secondary = IosGreen,
    onSecondary = Color.White,
    secondaryContainer = Color(0xFFE8F8ED),
    onSecondaryContainer = Color(0xFF0F5123),
    tertiary = IosOrange,
    onTertiary = Color.White,
    tertiaryContainer = Color(0xFFFFF3E0),
    onTertiaryContainer = Color(0xFF663C00),
    error = IosRed,
    onError = Color.White,
    errorContainer = Color(0xFFFFEBEA),
    onErrorContainer = Color(0xFF660B05),
    background = IosLightBackground,
    onBackground = IosLightPrimaryText,
    surface = IosLightSurface,
    onSurface = IosLightPrimaryText,
    surfaceVariant = IosLightSecondarySurface,
    onSurfaceVariant = IosLightSecondaryText,
    outline = IosLightDivider
)

@Composable
fun MyApplicationTheme(
    darkTheme: Boolean = isSystemInDarkTheme(),
    content: @Composable () -> Unit
) {
    val colorScheme = if (darkTheme) DarkColorScheme else LightColorScheme

    MaterialTheme(
        colorScheme = colorScheme,
        typography = Typography,
        content = content
    )
}
