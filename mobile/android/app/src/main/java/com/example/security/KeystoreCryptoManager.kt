package com.example.security

import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import java.nio.ByteBuffer
import java.security.KeyStore
import java.security.SecureRandom
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec

/**
 * Provides hardware-backed AES-256-GCM encryption via Android Keystore for:
 * 1. Notification text payloads stored in the local Room SQLite queue.
 * 2. Device-scoped bearer token stored in local preferences.
 *
 * Includes a real AES-256-GCM software fallback key exclusively when running inside
 * local JVM / Robolectric unit tests where the AndroidKeyStore provider is not mounted.
 */
class KeystoreCryptoManager(
    private val keyAlias: String = DEFAULT_KEY_ALIAS,
    private val allowInMemoryForTests: Boolean = false
) {
    private val secureRandom = SecureRandom()
    @Volatile
    private var jvmFallbackKey: SecretKey? = null

    fun encryptText(plainText: String): String {
        if (plainText.isEmpty()) return ""
        val secretKey = getOrCreateSecretKey()
        val cipher = Cipher.getInstance(TRANSFORMATION)
        return try {
            // AndroidKeyStore generates its own random IV on init(ENCRYPT_MODE, secretKey)
            cipher.init(Cipher.ENCRYPT_MODE, secretKey)
            val iv = cipher.iv
            val cipherBytes = cipher.doFinal(plainText.toByteArray(Charsets.UTF_8))
            packIvAndCiphertext(iv, cipherBytes)
        } catch (_: Exception) {
            // Software SecretKeySpec fallback requires an explicit 12-byte GCM IV
            val iv = ByteArray(GCM_IV_LENGTH_BYTES).also { secureRandom.nextBytes(it) }
            val spec = GCMParameterSpec(GCM_TAG_LENGTH_BITS, iv)
            cipher.init(Cipher.ENCRYPT_MODE, secretKey, spec)
            val cipherBytes = cipher.doFinal(plainText.toByteArray(Charsets.UTF_8))
            packIvAndCiphertext(iv, cipherBytes)
        }
    }

    fun decryptText(encodedBlob: String): String {
        if (encodedBlob.isEmpty()) return ""
        val raw = Base64.decode(encodedBlob, Base64.NO_WRAP)
        require(raw.size > GCM_IV_LENGTH_BYTES + 1) { "Invalid encrypted blob length" }

        val buffer = ByteBuffer.wrap(raw)
        val ivLength = buffer.int
        require(ivLength in 12..16 && raw.size >= 4 + ivLength) { "Corrupted GCM IV header" }

        val iv = ByteArray(ivLength)
        buffer.get(iv)
        val cipherBytes = ByteArray(buffer.remaining())
        buffer.get(cipherBytes)

        val secretKey = getOrCreateSecretKey()
        val cipher = Cipher.getInstance(TRANSFORMATION)
        val spec = GCMParameterSpec(GCM_TAG_LENGTH_BITS, iv)
        cipher.init(Cipher.DECRYPT_MODE, secretKey, spec)
        val plainBytes = cipher.doFinal(cipherBytes)
        return String(plainBytes, Charsets.UTF_8)
    }

    /**
     * Rotates/destroys the current encryption key on device disconnect or revocation.
     */
    @Synchronized
    fun destroyAndRotateKey() {
        try {
            val keyStore = KeyStore.getInstance(ANDROID_KEYSTORE)
            keyStore.load(null)
            if (keyStore.containsAlias(keyAlias)) {
                keyStore.deleteEntry(keyAlias)
            }
        } catch (_: Exception) {
            check(allowInMemoryForTests) { "Android Keystore unavailable" }
        }
        val newBytes = ByteArray(32).also { secureRandom.nextBytes(it) }
        jvmFallbackKey = SecretKeySpec(newBytes, "AES")
    }

    private fun packIvAndCiphertext(iv: ByteArray, cipherBytes: ByteArray): String {
        val buffer = ByteBuffer.allocate(4 + iv.size + cipherBytes.size)
        buffer.putInt(iv.size)
        buffer.put(iv)
        buffer.put(cipherBytes)
        return Base64.encodeToString(buffer.array(), Base64.NO_WRAP)
    }

    @Synchronized
    private fun getOrCreateSecretKey(): SecretKey {
        try {
            val keyStore = KeyStore.getInstance(ANDROID_KEYSTORE)
            keyStore.load(null)
            val existingEntry = keyStore.getEntry(keyAlias, null) as? KeyStore.SecretKeyEntry
            if (existingEntry != null) {
                return existingEntry.secretKey
            }

            val keyGenerator = KeyGenerator.getInstance(
                KeyProperties.KEY_ALGORITHM_AES,
                ANDROID_KEYSTORE
            )
            val parameterSpec = KeyGenParameterSpec.Builder(
                keyAlias,
                KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT
            )
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setKeySize(256)
                .setRandomizedEncryptionRequired(true)
                .build()

            keyGenerator.init(parameterSpec)
            return keyGenerator.generateKey()
        } catch (_: Exception) {
            check(allowInMemoryForTests) { "Android Keystore unavailable" }
            val current = jvmFallbackKey
            if (current != null) return current
            val bytes = ByteArray(32).also { secureRandom.nextBytes(it) }
            return SecretKeySpec(bytes, "AES").also { jvmFallbackKey = it }
        }
    }

    companion object {
        private const val ANDROID_KEYSTORE = "AndroidKeyStore"
        private const val TRANSFORMATION = "AES/GCM/NoPadding"
        private const val GCM_IV_LENGTH_BYTES = 12
        private const val GCM_TAG_LENGTH_BITS = 128
        const val DEFAULT_KEY_ALIAS = "butler_companion_aes256_master_key"
    }
}
