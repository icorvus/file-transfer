#!/usr/bin/env python3
"""Generate the committed Xcode project using only Python's standard library."""
from pathlib import Path
import hashlib
import json

ROOT = Path(__file__).resolve().parents[1] / "ios"
objects = {}

def uid(label):
    return hashlib.sha256(label.encode()).hexdigest()[:24].upper()

def add(label, isa, **values):
    key = uid(label)
    objects[key] = dict(isa=isa, **values)
    return key

def ref(path, kind):
    return add("file:" + path, "PBXFileReference", lastKnownFileType=kind, path=path, sourceTree="<group>")

def build(label, file, **extra):
    return add("build:" + label, "PBXBuildFile", fileRef=file, **extra)

shared = [ref(str(p.relative_to(ROOT)), "sourcecode.swift") for p in sorted((ROOT / "Shared").glob("*.swift"))]
app = [ref(str(p.relative_to(ROOT)), "sourcecode.swift") for p in sorted((ROOT / "NearDrop").glob("*.swift"))]
extension = [ref(str(p.relative_to(ROOT)), "sourcecode.swift") for p in sorted((ROOT / "ShareExtension").glob("*.swift"))]
tests = [ref(str(p.relative_to(ROOT)), "sourcecode.swift") for p in sorted((ROOT / "Tests").glob("*.swift"))]
config = ref("Config.xcconfig", "text.xcconfig")
privacy = ref("Shared/PrivacyInfo.xcprivacy", "text.xml")
assets = ref("NearDrop/Assets.xcassets", "folder.assetcatalog")
extras = [ref("NearDrop/Info.plist", "text.plist.xml"), ref("ShareExtension/Info.plist", "text.plist.xml"), ref("Shared/NearDrop.entitlements", "text.plist.entitlements")]

products = {}
for target, suffix, kind in [("NearDrop", "app", "wrapper.application"), ("NearDropShare", "appex", "wrapper.app-extension"), ("NearDropTests", "xctest", "wrapper.cfbundle")]:
    products[target] = add("product:" + target, "PBXFileReference", explicitFileType=kind, includeInIndex="0", path=target+"."+suffix, sourceTree="BUILT_PRODUCTS_DIR")

product_group = add("products", "PBXGroup", children=list(products.values()), name="Products", sourceTree="<group>")
main_group = add("main", "PBXGroup", children=shared+app+extension+tests+[config,privacy,assets]+extras+[product_group], sourceTree="<group>")

def configs(label, settings, base=False):
    result = []
    for mode in ["Debug", "Release"]:
        values = dict(settings)
        if label == "project":
            values.update(SWIFT_OPTIMIZATION_LEVEL="-Onone" if mode == "Debug" else "-O", DEBUG_INFORMATION_FORMAT="dwarf" if mode == "Debug" else "dwarf-with-dsym")
            if mode == "Debug": values.update(ENABLE_TESTABILITY="YES", SWIFT_ACTIVE_COMPILATION_CONDITIONS="DEBUG $(inherited)")
        extra = dict(baseConfigurationReference=config) if base else {}
        result.append(add("config:"+label+mode, "XCBuildConfiguration", buildSettings=values, name=mode, **extra))
    return add("configs:"+label, "XCConfigurationList", buildConfigurations=result, defaultConfigurationIsVisible="0", defaultConfigurationName="Release")

project_config = configs("project", dict(IPHONEOS_DEPLOYMENT_TARGET="26.0", SDKROOT="iphoneos", SWIFT_VERSION="6.0", SWIFT_STRICT_CONCURRENCY="complete", SWIFT_APPROACHABLE_CONCURRENCY="YES", SWIFT_DEFAULT_ACTOR_ISOLATION="MainActor", CLANG_ENABLE_MODULES="YES", CODE_SIGN_STYLE="Automatic", TARGETED_DEVICE_FAMILY="1", SUPPORTED_PLATFORMS="iphoneos iphonesimulator", SUPPORTS_MACCATALYST="NO", ENABLE_USER_SCRIPT_SANDBOXING="YES", GCC_C_LANGUAGE_STANDARD="gnu17", CLANG_ENABLE_OBJC_ARC="YES"), base=True)

proxy = add("share-proxy", "PBXContainerItemProxy", containerPortal=uid("project"), proxyType="1", remoteGlobalIDString=uid("target:NearDropShare"), remoteInfo="NearDropShare")
dependency = add("share-dependency", "PBXTargetDependency", target=uid("target:NearDropShare"), targetProxy=proxy)
app_proxy = add("app-proxy", "PBXContainerItemProxy", containerPortal=uid("project"), proxyType="1", remoteGlobalIDString=uid("target:NearDrop"), remoteInfo="NearDrop")
app_dependency = add("app-dependency", "PBXTargetDependency", target=uid("target:NearDrop"), targetProxy=app_proxy)

for target, sources, kind in [("NearDrop",shared+app,"com.apple.product-type.application"), ("NearDropShare",shared+extension,"com.apple.product-type.app-extension"), ("NearDropTests",tests,"com.apple.product-type.bundle.unit-test")]:
    source_phase = add("sources:"+target,"PBXSourcesBuildPhase",buildActionMask="2147483647",files=[build(target+f,f) for f in sources],runOnlyForDeploymentPostprocessing="0")
    resources = [] if target == "NearDropTests" else [build(target+"privacy",privacy)]
    if target == "NearDrop": resources.append(build(target+"assets",assets))
    resource_phase = add("resources:"+target,"PBXResourcesBuildPhase",buildActionMask="2147483647",files=resources,runOnlyForDeploymentPostprocessing="0")
    framework_phase = add("frameworks:"+target,"PBXFrameworksBuildPhase",buildActionMask="2147483647",files=[],runOnlyForDeploymentPostprocessing="0")
    phases = [source_phase,framework_phase,resource_phase]
    settings = dict(PRODUCT_NAME="$(TARGET_NAME)", CURRENT_PROJECT_VERSION="1", MARKETING_VERSION="1.0")
    dependencies = []
    if target == "NearDrop":
        settings.update(PRODUCT_BUNDLE_IDENTIFIER="$(NEARDROP_BUNDLE_PREFIX)", INFOPLIST_FILE="NearDrop/Info.plist", CODE_SIGN_ENTITLEMENTS="Shared/NearDrop.entitlements", ASSETCATALOG_COMPILER_APPICON_NAME="AppIcon", ASSETCATALOG_COMPILER_GLOBAL_ACCENT_COLOR_NAME="AccentColor")
        phases.append(add("embed","PBXCopyFilesBuildPhase",buildActionMask="2147483647",dstPath="",dstSubfolderSpec="13",files=[build("embed-share",products["NearDropShare"],settings={"ATTRIBUTES":["RemoveHeadersOnCopy"]})],name="Embed App Extensions",runOnlyForDeploymentPostprocessing="0"))
        dependencies = [dependency]
    elif target == "NearDropShare":
        settings.update(PRODUCT_BUNDLE_IDENTIFIER="$(NEARDROP_BUNDLE_PREFIX).share", INFOPLIST_FILE="ShareExtension/Info.plist", CODE_SIGN_ENTITLEMENTS="Shared/NearDrop.entitlements", APPLICATION_EXTENSION_API_ONLY="YES", SKIP_INSTALL="YES")
    else:
        settings.update(PRODUCT_BUNDLE_IDENTIFIER="$(NEARDROP_BUNDLE_PREFIX).tests", GENERATE_INFOPLIST_FILE="YES", TEST_HOST="$(BUILT_PRODUCTS_DIR)/NearDrop.app/$(BUNDLE_EXECUTABLE_FOLDER_PATH)/NearDrop", BUNDLE_LOADER="$(TEST_HOST)")
        dependencies = [app_dependency]
    add("target:"+target,"PBXNativeTarget",buildConfigurationList=configs(target,settings),buildPhases=phases,buildRules=[],dependencies=dependencies,name=target,productName=target,productReference=products[target],productType=kind)

add("project","PBXProject",attributes={"BuildIndependentTargetsInParallel":"YES","LastUpgradeCheck":"2600","TargetAttributes":{uid("target:"+name):{"CreatedOnToolsVersion":"26.0","ProvisioningStyle":"Automatic"} for name in products}},buildConfigurationList=project_config,compatibilityVersion="Xcode 15.0",developmentRegion="en",hasScannedForEncodings="0",knownRegions=["en","Base"],mainGroup=main_group,productRefGroup=product_group,projectDirPath="",projectRoot="",targets=[uid("target:"+name) for name in products])

def encode(value, depth=0):
    pad="\t"*depth
    if isinstance(value,dict): return "{\n"+"".join(pad+"\t"+json.dumps(k)+" = "+encode(v,depth+1)+";\n" for k,v in value.items())+pad+"}"
    if isinstance(value,list): return "(\n"+"".join(pad+"\t"+encode(v,depth+1)+",\n" for v in value)+pad+")"
    return json.dumps(str(value))

project=ROOT/"NearDrop.xcodeproj"
project.mkdir(exist_ok=True)
(project/"project.pbxproj").write_text("// !$*UTF8*$!\n"+encode(dict(archiveVersion="1",classes={},objectVersion="60",objects=objects,rootObject=uid("project")))+"\n")
scheme=project/"xcshareddata"/"xcschemes"
scheme.mkdir(parents=True,exist_ok=True)
def reference(name, extension):
    return f'<BuildableReference BuildableIdentifier="primary" BlueprintIdentifier="{uid("target:"+name)}" BuildableName="{name}.{extension}" BlueprintName="{name}" ReferencedContainer="container:NearDrop.xcodeproj"/>'
(scheme/"NearDrop.xcscheme").write_text(f'''<?xml version="1.0" encoding="UTF-8"?>
<Scheme LastUpgradeVersion="2600" version="1.3">
  <BuildAction parallelizeBuildables="YES" buildImplicitDependencies="YES"><BuildActionEntries><BuildActionEntry buildForTesting="YES" buildForRunning="YES" buildForProfiling="YES" buildForArchiving="YES" buildForAnalyzing="YES">{reference("NearDrop","app")}</BuildActionEntry></BuildActionEntries></BuildAction>
  <TestAction buildConfiguration="Debug" selectedDebuggerIdentifier="Xcode.DebuggerFoundation.Debugger.LLDB" selectedLauncherIdentifier="Xcode.IDEFoundation.Launcher.LLDB" shouldUseLaunchSchemeArgsEnv="YES"><Testables><TestableReference skipped="NO">{reference("NearDropTests","xctest")}</TestableReference></Testables></TestAction>
  <LaunchAction buildConfiguration="Debug" selectedDebuggerIdentifier="Xcode.DebuggerFoundation.Debugger.LLDB" selectedLauncherIdentifier="Xcode.IDEFoundation.Launcher.LLDB" launchStyle="0" useCustomWorkingDirectory="NO" ignoresPersistentStateOnLaunch="NO" debugDocumentVersioning="YES" debugServiceExtension="internal" allowLocationSimulation="YES"><BuildableProductRunnable runnableDebuggingMode="0">{reference("NearDrop","app")}</BuildableProductRunnable></LaunchAction>
  <ProfileAction buildConfiguration="Release" shouldUseLaunchSchemeArgsEnv="YES" savedToolIdentifier="" useCustomWorkingDirectory="NO" debugDocumentVersioning="YES"><BuildableProductRunnable runnableDebuggingMode="0">{reference("NearDrop","app")}</BuildableProductRunnable></ProfileAction>
  <AnalyzeAction buildConfiguration="Debug"/><ArchiveAction buildConfiguration="Release" revealArchiveInOrganizer="YES"/>
</Scheme>''')
print(project)
